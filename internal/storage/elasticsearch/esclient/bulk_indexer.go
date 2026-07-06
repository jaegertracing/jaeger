// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package esclient

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore/spanstoremetrics"
)

const (
	// maxBulkRetries bounds per-item retries so a persistently failing document
	// can't be re-sent forever.
	maxBulkRetries = 3
	// bulkRetryBase is the base delay for exponential backoff between retries.
	bulkRetryBase = 50 * time.Millisecond
)

// retryableBulkStatus reports whether a per-item bulk status warrants a retry:
// request timeout (408), too many requests (429), unavailable (503), or
// insufficient storage (507) — transient conditions where re-sending the item
// (rather than the whole batch) is safe.
func retryableBulkStatus(status int) bool {
	switch status {
	case http.StatusRequestTimeout, http.StatusTooManyRequests,
		http.StatusServiceUnavailable, http.StatusInsufficientStorage:
		return true
	default:
		return false
	}
}

// BulkIndexerConfig bounds a BulkIndexer's buffering and concurrency.
type BulkIndexerConfig struct {
	FlushBytes    int           // flush before a batch exceeds this many bytes (0 ⇒ unbounded)
	MaxActions    int           // flush once this many documents are buffered (0 ⇒ unbounded)
	FlushInterval time.Duration // flush a partial batch after this long (0 ⇒ no timer)
	Workers       int           // max concurrent in-flight _bulk requests (min 1)
}

// bulkDoc is one encoded document (an action + source NDJSON line pair) plus the
// context needed to report an error for it.
type bulkDoc struct {
	payload []byte
	index   string
}

// BulkIndexer buffers documents and writes them to _bulk in bounded batches over
// the shared transport. Unlike olivere's BulkProcessor it enforces a hard byte
// ceiling — it flushes before a batch would exceed FlushBytes, so no single
// request can grow unbounded (#2192) — and retries individual failed items
// (408/429/503/507) rather than replaying the whole batch, which would duplicate
// successful writes.
type BulkIndexer struct {
	client     Client
	typed      bool // whether the backend emits _type (ES6)
	flushBytes int
	maxActions int
	sem        chan struct{} // bounds concurrent in-flight requests (Workers)
	metrics    *spanstoremetrics.WriteMetrics
	logger     *zap.Logger

	mu           sync.Mutex
	pending      []bulkDoc
	pendingBytes int
	closed       bool

	ticker *time.Ticker
	done   chan struct{}
	wg     sync.WaitGroup
}

var _ BulkWriter = (*BulkIndexer)(nil)

// NewBulkIndexer returns a running BulkIndexer. Call Close to flush and stop it.
func NewBulkIndexer(client Client, cfg BulkIndexerConfig, metricsFactory metrics.Factory, logger *zap.Logger) *BulkIndexer {
	workers := max(cfg.Workers, 1)
	b := &BulkIndexer{
		client:     client,
		typed:      client.version.SupportsTypedIndices(),
		flushBytes: cfg.FlushBytes,
		maxActions: cfg.MaxActions,
		sem:        make(chan struct{}, workers),
		metrics:    spanstoremetrics.NewWriter(metricsFactory, "bulk_index"),
		logger:     logger,
		done:       make(chan struct{}),
	}
	if cfg.FlushInterval > 0 {
		b.ticker = time.NewTicker(cfg.FlushInterval)
		b.wg.Add(1)
		go b.flushLoop()
	}
	return b
}

// Add encodes and buffers a document, flushing the current batch first if this
// document would push it past the byte or action limit.
func (b *BulkIndexer) Add(item BulkItem) {
	var buf bytes.Buffer
	if err := encodeBulkItem(&buf, item, b.typed); err != nil {
		b.logger.Error("failed to encode bulk document",
			zap.String("index", item.Index), zap.Error(err))
		return
	}
	doc := bulkDoc{payload: buf.Bytes(), index: item.Index}

	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		b.logger.Warn("bulk document added after Close; dropping", zap.String("index", item.Index))
		return
	}
	var batch []bulkDoc
	if len(b.pending) > 0 && (b.exceedsBytesLocked(len(doc.payload)) || b.reachedMaxActionsLocked()) {
		batch = b.takePendingLocked()
	}
	b.pending = append(b.pending, doc)
	b.pendingBytes += len(doc.payload)
	b.mu.Unlock()

	if batch != nil {
		b.send(batch)
	}
}

// Close flushes any buffered documents and stops the flush timer. It is
// idempotent; documents added after Close are dropped.
func (b *BulkIndexer) Close() error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil
	}
	b.closed = true
	b.mu.Unlock()

	if b.ticker != nil {
		b.ticker.Stop()
		close(b.done)
		b.wg.Wait()
	}
	b.flushReady()
	return nil
}

func (b *BulkIndexer) flushLoop() {
	defer b.wg.Done()
	for {
		select {
		case <-b.ticker.C:
			b.flushReady()
		case <-b.done:
			return
		}
	}
}

// flushReady sends whatever is currently buffered.
func (b *BulkIndexer) flushReady() {
	b.mu.Lock()
	batch := b.takePendingLocked()
	b.mu.Unlock()
	if len(batch) > 0 {
		b.send(batch)
	}
}

func (b *BulkIndexer) exceedsBytesLocked(n int) bool {
	return b.flushBytes > 0 && b.pendingBytes+n > b.flushBytes
}

func (b *BulkIndexer) reachedMaxActionsLocked() bool {
	return b.maxActions > 0 && len(b.pending) >= b.maxActions
}

func (b *BulkIndexer) takePendingLocked() []bulkDoc {
	if len(b.pending) == 0 {
		return nil
	}
	batch := b.pending
	b.pending = nil
	b.pendingBytes = 0
	return batch
}

// send posts docs to _bulk and retries the individual items that failed with a
// retryable status, using exponential backoff and holding a worker slot for the
// duration.
func (b *BulkIndexer) send(docs []bulkDoc) {
	b.sem <- struct{}{}
	defer func() { <-b.sem }()

	for attempt := 0; ; attempt++ {
		var body bytes.Buffer
		for _, d := range docs {
			body.Write(d.payload)
		}
		start := time.Now()
		raw, err := b.client.request(context.Background(), elasticRequest{
			endpoint:    "_bulk",
			method:      http.MethodPost,
			body:        body.Bytes(),
			contentType: "application/x-ndjson",
		})
		latency := time.Since(start)

		if err != nil {
			b.metrics.LatencyErr.Record(latency)
			b.metrics.Attempts.Inc(int64(len(docs)))
			b.metrics.Errors.Inc(int64(len(docs)))
			b.logger.Error("Elasticsearch could not process bulk request",
				zap.Int("request_count", len(docs)), zap.Error(err))
			if attempt < maxBulkRetries {
				time.Sleep(bulkRetryBase << attempt)
				continue
			}
			return
		}
		b.metrics.LatencyOk.Record(latency)

		var resp bulkResponse
		if uerr := json.Unmarshal(raw, &resp); uerr != nil {
			b.logger.Error("failed to parse bulk response", zap.Error(uerr))
			b.metrics.Attempts.Inc(int64(len(docs)))
			b.metrics.Errors.Inc(int64(len(docs)))
			return
		}

		retry, failed := b.classify(docs, resp, attempt)
		b.metrics.Attempts.Inc(int64(len(docs)))
		b.metrics.Inserts.Inc(int64(len(docs) - failed - len(retry)))
		b.metrics.Errors.Inc(int64(failed))

		if len(retry) == 0 {
			return
		}
		docs = retry
		time.Sleep(bulkRetryBase << attempt)
	}
}

// classify splits a bulk response into the documents to retry and a count of
// permanently failed ones, logging each failure. Successful documents are
// neither retried nor counted as failed.
func (b *BulkIndexer) classify(docs []bulkDoc, resp bulkResponse, attempt int) (retry []bulkDoc, failed int) {
	for i, item := range resp.Items {
		if i >= len(docs) {
			break
		}
		status, itemErr := item.result()
		switch {
		case retryableBulkStatus(status) && attempt < maxBulkRetries:
			retry = append(retry, docs[i])
		case itemErr != nil || status < 200 || status >= 300:
			failed++
			b.logger.Error("Elasticsearch part of bulk request failed",
				zap.String("index", docs[i].index),
				zap.Int("status", status),
				zap.ByteString("error", itemErr))
		default:
			// success: the document was written, so it is neither retried nor failed.
		}
	}
	return retry, failed
}

// bulkResponse is the subset of the _bulk reply we act on: the per-item status.
type bulkResponse struct {
	Errors bool               `json:"errors"`
	Items  []bulkResponseItem `json:"items"`
}

// bulkResponseItem is keyed by the action verb ("index"/"create"); each holds
// that item's HTTP status and optional error object.
type bulkResponseItem map[string]struct {
	Status int             `json:"status"`
	Error  json.RawMessage `json:"error"`
}

func (it bulkResponseItem) result() (status int, itemErr json.RawMessage) {
	for _, v := range it {
		return v.Status, v.Error
	}
	return 0, nil
}
