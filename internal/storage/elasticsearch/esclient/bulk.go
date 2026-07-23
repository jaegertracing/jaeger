// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package esclient

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/elastic/go-elasticsearch/v9/esutil"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/metrics"
	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore/spanstoremetrics"
)

// BulkItem is a single document to write via the bulk API.
type BulkItem struct {
	Index  string         // target index, alias, or data stream
	ID     string         // optional document _id (empty ⇒ server-generated)
	OpType es.WriteOpType // "index" (default) or "create", depending on rotation strategy
	Body   any            // source document; a json.RawMessage (must be a valid JSON document) is written verbatim, anything else is json.Marshal'd
}

// BulkIndexerConfig bounds a BulkIndexer's buffering and concurrency.
type BulkIndexerConfig struct {
	FlushBytes    int           // flush once a worker's buffer reaches this many bytes
	FlushInterval time.Duration // flush a partial buffer after this long
	Workers       int           // number of concurrent flush workers
}

// BulkIndexer writes documents to _bulk over the shared transport. It wraps the
// official go-elasticsearch esutil.BulkIndexer — a bounded, worker-pooled indexer
// that flushes on a byte threshold or interval (fixing #2192) — driven by our own
// transport pool (via the esapi.Transport interface), so no product-checked
// go-elasticsearch client is involved.
type BulkIndexer struct {
	bi      esutil.BulkIndexer
	metrics *spanstoremetrics.WriteMetrics
	logger  *zap.Logger
}

var _ BulkWriter = (*BulkIndexer)(nil)

// newESUtilBulkIndexer wraps esutil.NewBulkIndexer in an overridable package var
// so tests can exercise the constructor error path (which real esutil only
// returns for a nil Client, and we always pass a non-nil one).
var newESUtilBulkIndexer = esutil.NewBulkIndexer

// flushState tracks one in-flight flush. esutil's OnFlushEnd callback carries no
// success/failure signal, so OnError flips failed and OnFlushEnd reads it to
// record the flush latency under latency-ok or latency-err. A pointer is stored
// in the flush context (esutil runs a flush's callbacks on one goroutine, so the
// read/write need no synchronization) and is scoped to that single flush.
type flushState struct {
	start  time.Time
	failed bool
}

type flushStateKey struct{}

// NewBulkIndexer returns a running BulkIndexer. The caller owns its lifecycle and
// must call Close to flush buffered documents and stop the workers.
func NewBulkIndexer(client *Client, cfg BulkIndexerConfig, metricsFactory metrics.Factory, logger *zap.Logger) (*BulkIndexer, error) {
	b := &BulkIndexer{
		metrics: spanstoremetrics.NewWriter(metricsFactory, "bulk_index"),
		logger:  logger,
	}
	// Default to a single worker when the config doesn't set one, matching the
	// historical BulkProcessor default; esutil would otherwise fan out to
	// NumCPU workers.
	workers := cfg.Workers
	if workers <= 0 {
		workers = 1
	}
	bi, err := newESUtilBulkIndexer(esutil.BulkIndexerConfig{
		// Client is the esapi.Transport esutil sends _bulk requests through; our
		// Client satisfies it (Perform delegates down through rawClient to the
		// pool), so esutil runs on our transport — the same multi-node pool and
		// auth/TLS/SigV4 stack every request uses — not a go-elasticsearch client.
		Client:        client,
		NumWorkers:    workers,
		FlushBytes:    cfg.FlushBytes,
		FlushInterval: cfg.FlushInterval,
		OnError:       b.onFlushError,
		OnFlushStart:  b.onFlushStart,
		OnFlushEnd:    b.onFlushEnd,
	})
	if err != nil {
		return nil, err
	}
	b.bi = bi
	return b, nil
}

// onFlushStart stamps the flush start time into the context so onFlushEnd can
// measure latency and onFlushError can flag the flush as failed.
func (*BulkIndexer) onFlushStart(ctx context.Context) context.Context {
	return context.WithValue(ctx, flushStateKey{}, &flushState{start: time.Now()})
}

// onFlushError handles a whole-request flush failure (transport error or non-2xx
// _bulk response). It flags the flush so onFlushEnd records latency-err; the
// per-item error counts are handled in onItemFailure, which esutil also invokes
// for every item of a failed flush.
func (b *BulkIndexer) onFlushError(ctx context.Context, err error) {
	if st, ok := ctx.Value(flushStateKey{}).(*flushState); ok {
		st.failed = true
	}
	b.logger.Error("bulk indexer error", zap.Error(err))
}

// onFlushEnd records the flush latency once per flush, under latency-err when the
// whole flush failed and latency-ok otherwise.
func (b *BulkIndexer) onFlushEnd(ctx context.Context) {
	st, ok := ctx.Value(flushStateKey{}).(*flushState)
	if !ok {
		return
	}
	if st.failed {
		b.metrics.LatencyErr.Record(time.Since(st.start))
		return
	}
	b.metrics.LatencyOk.Record(time.Since(st.start))
}

// Add encodes and enqueues a document. Encoding or enqueue failures are logged
// and counted; the bulk API's own semantics (buffering, flush, per-item results)
// are handled by esutil.
func (b *BulkIndexer) Add(item BulkItem) {
	body, err := encodeBody(item.Body)
	if err != nil {
		b.recordDropped(item.Index, "failed to encode bulk document", err)
		return
	}
	action := string(item.OpType)
	if action == "" {
		action = string(es.WriteOpIndex)
	}
	err = b.bi.Add(context.Background(), esutil.BulkIndexerItem{
		Action:     action,
		Index:      item.Index,
		DocumentID: item.ID,
		Body:       bytes.NewReader(body),
		OnSuccess: func(context.Context, esutil.BulkIndexerItem, esutil.BulkIndexerResponseItem) {
			b.onItemSuccess()
		},
		OnFailure: func(_ context.Context, _ esutil.BulkIndexerItem, resp esutil.BulkIndexerResponseItem, itemErr error) {
			if resp.Status == http.StatusConflict {
				// A 409 version_conflict on a document with our deterministic _id
				// (op_type: create, used by data streams) means the byte-identical
				// span is already stored — the expected outcome of an at-least-once
				// retry, not a failure. Treat it as an idempotent success rather than
				// logging a spurious error. See RFC 0007 §4.7.
				b.onItemConflict(item.Index)
				return
			}
			b.onItemFailure(item.Index, resp.Status, itemErr)
		},
	})
	if err != nil {
		// Unreachable with our inputs (a background context and a *bytes.Reader
		// body never make esutil's Add fail), but a full queue or closed indexer
		// would land here; treat it as a dropped document rather than ignore it.
		b.recordDropped(item.Index, "failed to enqueue bulk document", err)
	}
}

// encodeBody returns the JSON document bytes for a bulk item. A json.RawMessage is
// already-encoded JSON, returned verbatim: this lets the span hot path marshal the
// document once, hash those bytes for the _id, and hand them straight to the bulk
// request without a second reflection-heavy encode. Any other value is marshaled.
func encodeBody(doc any) ([]byte, error) {
	if raw, ok := doc.(json.RawMessage); ok {
		return raw, nil
	}
	return json.Marshal(doc)
}

// onItemSuccess counts one successfully indexed document.
func (b *BulkIndexer) onItemSuccess() {
	b.metrics.Attempts.Inc(1)
	b.metrics.Inserts.Inc(1)
}

// onItemFailure counts one document that the server rejected. esutil calls it
// both for a per-item error inside an otherwise-OK flush and for every item of a
// whole-request flush failure (its notifyItemsOnError fans a transport/non-2xx
// error out to each item), so both cases are counted here. Flush latency is
// recorded once per flush in onFlushEnd (latency-err on whole failure).
func (b *BulkIndexer) onItemFailure(index string, status int, err error) {
	b.metrics.Attempts.Inc(1)
	b.metrics.Errors.Inc(1)
	b.logger.Error("Elasticsearch part of bulk request failed",
		zap.String("index", index), zap.Int("status", status), zap.Error(err))
}

// onItemConflict counts a document rejected with a 409 version_conflict as a
// successful (idempotent) write: with a deterministic _id the conflicting document
// is already durably stored, so the write achieved its goal. Counted like a normal
// insert and logged at debug, not as an error (see the OnFailure handler).
func (b *BulkIndexer) onItemConflict(index string) {
	b.metrics.Attempts.Inc(1)
	b.metrics.Inserts.Inc(1)
	b.logger.Debug("Elasticsearch bulk item already present (idempotent write)",
		zap.String("index", index), zap.Int("status", http.StatusConflict))
}

// recordDropped counts a document that never reached the server — dropped before
// enqueue because it could not be encoded or accepted — as a failed attempt.
func (b *BulkIndexer) recordDropped(index, reason string, err error) {
	b.metrics.Attempts.Inc(1)
	b.metrics.Errors.Inc(1)
	b.logger.Error(reason, zap.String("index", index), zap.Error(err))
}

// Close flushes buffered documents and stops the workers.
func (b *BulkIndexer) Close() error {
	return b.bi.Close(context.Background())
}
