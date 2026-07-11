// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package esclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/metrics"
	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore/spanstoremetrics"
)

// defaultSyncBulkMaxBytes bounds a single _bulk chunk when the config leaves
// maxBytes unset. It mirrors the async indexer's 5 MB default.
const defaultSyncBulkMaxBytes = 5 * 1024 * 1024

const (
	// maxReportedFailures caps how many rejected items' reasons are rendered into
	// the returned error. A whole-batch rejection (e.g. the backend is down) can
	// fail every item, so without a cap the error string would be enormous; the
	// true rejected/total counts are always reported regardless.
	maxReportedFailures = 20
	// maxErrorPayloadBytes bounds each rendered per-item error object, which the
	// backend can return arbitrarily large.
	maxErrorPayloadBytes = 256
)

// SyncBulkWriter issues synchronous, size-bounded _bulk requests over the shared
// transport. Unlike the async BulkIndexer — which enqueues into esutil and reports
// failures only through callbacks — Bulk blocks until the backend has durably
// acknowledged the batch and returns a real error on a transport failure or any
// item-level rejection. It is the write primitive RFC 0007's synchronous mode
// needs; it is a peer of BulkIndexer over the same Client, not a method on it.
// (esutil's BulkIndexer does have a blocking Flush, but it reports per-item
// outcomes only through OnSuccess/OnFailure callbacks over a shared, worker-pooled
// buffer — Flush itself returns only transport-level errors — so it yields no clean
// synchronous per-batch verdict; one direct _bulk round-trip does.)
type SyncBulkWriter struct {
	client   *Client
	maxBytes int
	metrics  *spanstoremetrics.WriteMetrics
	logger   *zap.Logger
}

// NewSyncBulkWriter returns a SyncBulkWriter that sends each _bulk chunk over the
// given Client. maxBytes caps a chunk client-side (defaulting when non-positive)
// and should stay well under the backend's own request limit: ES/OS reject a
// body larger than http.max_content_length (default 100 MB) with 413. The cap
// bounds only the assembled chunk; a single document exceeding maxBytes cannot be
// split, so it is sent alone and may still hit that server limit (§4.4).
func NewSyncBulkWriter(client *Client, maxBytes int, metricsFactory metrics.Factory, logger *zap.Logger) *SyncBulkWriter {
	if maxBytes <= 0 {
		maxBytes = defaultSyncBulkMaxBytes
	}
	return &SyncBulkWriter{
		client:   client,
		maxBytes: maxBytes,
		metrics:  spanstoremetrics.NewWriter(metricsFactory, "bulk_index"),
		logger:   logger,
	}
}

// Bulk writes every item in one or more synchronous _bulk requests, each bounded
// to maxBytes, and returns an error if the transport failed or any item was
// rejected. Chunks are sent in sequence and their errors joined. On error the
// caller re-sends the whole batch (Kafka re-delivery / exporter retry). Retry is
// not idempotent for spans: a span document carries no _id, so ES/OS assigns a
// fresh one and a re-sent span becomes a duplicate — today's behavior on any
// retry. A document with a deterministic _id (the service:operation dedup doc)
// instead upserts and never duplicates. A single item larger than maxBytes is
// still sent in a chunk of its own — the backend, not the client, decides whether
// it fits (a 413 then surfaces as a returned error).
func (w *SyncBulkWriter) Bulk(ctx context.Context, items []BulkItem) error {
	if len(items) == 0 {
		return nil
	}

	blobs := make([][]byte, len(items))
	for i := range items {
		blob, err := encodeBulkItem(items[i])
		if err != nil {
			// A span/service document is JSON-encodable, but a caller could pass a
			// Body that json.Marshal rejects (an unsupported or cyclic value), so
			// this is reachable; fail the whole batch rather than write it partially.
			w.metrics.Attempts.Inc(int64(len(items)))
			w.metrics.Errors.Inc(int64(len(items)))
			return fmt.Errorf("failed to encode bulk document for index %q: %w", items[i].Index, err)
		}
		blobs[i] = blob
	}

	var (
		errs      []error
		chunk     []byte
		chunkLen  int
		succeeded int
	)
	// flush is only ever called with a non-empty chunk: the empty batch returned
	// above, the in-loop flush is guarded by chunkLen > 0, and the final flush
	// runs after at least one item was appended.
	flush := func() {
		ok, err := w.sendChunk(ctx, chunk, chunkLen)
		succeeded += ok
		if err != nil {
			errs = append(errs, err)
		}
		chunk, chunkLen = nil, 0
	}
	for _, blob := range blobs {
		// Keep one item per chunk minimum: only split once the chunk is non-empty.
		if chunkLen > 0 && len(chunk)+len(blob) > w.maxBytes {
			flush()
		}
		chunk = append(chunk, blob...)
		chunkLen++
	}
	flush()

	w.metrics.Attempts.Inc(int64(len(items)))
	w.metrics.Inserts.Inc(int64(succeeded))
	w.metrics.Errors.Inc(int64(len(items) - succeeded))
	return errors.Join(errs...)
}

// sendChunk POSTs one NDJSON _bulk body and reports how many of its count items
// were durably indexed. A transport failure or non-2xx response yields (0, err);
// a 200 whose body flags per-item errors yields (count-failures, err).
func (w *SyncBulkWriter) sendChunk(ctx context.Context, body []byte, count int) (int, error) {
	start := time.Now()
	success := false
	defer func() {
		if success {
			w.metrics.LatencyOk.Record(time.Since(start))
		} else {
			w.metrics.LatencyErr.Record(time.Since(start))
		}
	}()

	raw, err := w.client.request(ctx, elasticRequest{
		endpoint:    "_bulk",
		method:      http.MethodPost,
		body:        body,
		contentType: "application/x-ndjson",
	})
	if err != nil {
		return 0, fmt.Errorf("bulk request failed: %w", err)
	}

	var resp bulkResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return 0, fmt.Errorf("failed to parse bulk response: %w", err)
	}
	// A well-formed _bulk response reports exactly one result per document, in
	// request order. If a proxy or partial response returns fewer (or more), the
	// per-item accounting below can't be trusted, so fail the whole chunk (0
	// durable) rather than silently miscount — the caller retries the batch.
	if len(resp.Items) != count {
		return 0, fmt.Errorf("malformed bulk response: %d item results for %d documents", len(resp.Items), count)
	}
	if !resp.Errors {
		success = true
		return count, nil
	}
	failed, sample := resp.failures()
	if failed == 0 {
		// errors:true but no item reported a failing status — nothing was actually
		// rejected, so the chunk is durable.
		success = true
		return count, nil
	}
	w.logger.Error("synchronous bulk write had rejected items",
		zap.Int("rejected", failed), zap.Int("total", count))
	msg := strings.Join(sample, "; ")
	if failed > len(sample) {
		msg += fmt.Sprintf("; …and %d more", failed-len(sample))
	}
	return count - failed, fmt.Errorf("%d of %d bulk items rejected: %s", failed, count, msg)
}

// encodeBulkItem renders one document as its two NDJSON lines: the action line
// ({"index":{"_index":…}} or {"create":…}) and the source line.
func encodeBulkItem(item BulkItem) ([]byte, error) {
	action := string(item.OpType)
	if action == "" {
		action = string(es.WriteOpIndex)
	}
	meta := map[string]map[string]string{action: {"_index": item.Index}}
	if item.ID != "" {
		meta[action]["_id"] = item.ID
	}
	// meta is a map of strings, which json.Marshal can never fail to encode, so
	// its error is ignored (there is no way to exercise the branch). item.Body
	// below is the caller-supplied document and can legitimately fail to encode.
	metaLine, _ := json.Marshal(meta)
	source, err := json.Marshal(item.Body)
	if err != nil {
		return nil, err
	}
	// Append without a precomputed capacity: summing the two lengths was flagged
	// as a possible allocation-size overflow. Letting append size the slice avoids
	// the arithmetic (documents vary in size — a span may carry a large payload).
	var blob []byte
	blob = append(blob, metaLine...)
	blob = append(blob, '\n')
	blob = append(blob, source...)
	blob = append(blob, '\n')
	return blob, nil
}

// bulkResponse is the subset of the _bulk response we act on: the top-level
// errors flag and each item's action-keyed result (status + optional error).
type bulkResponse struct {
	Errors bool                       `json:"errors"`
	Items  []map[string]bulkItemState `json:"items"`
}

type bulkItemState struct {
	Index  string          `json:"_index"`
	Status int             `json:"status"`
	Error  json.RawMessage `json:"error"`
}

// failures returns the number of rejected items (HTTP status ≥ 400 or a present
// error object) and a bounded, human-readable sample of their reasons — at most
// maxReportedFailures entries, each error payload truncated to maxErrorPayloadBytes
// — so the returned error stays small even when an entire large batch is rejected.
func (r bulkResponse) failures() (count int, sample []string) {
	for _, item := range r.Items {
		for _, state := range item { // one action entry per item
			if state.Status < http.StatusBadRequest && len(state.Error) == 0 {
				continue
			}
			count++
			if len(sample) >= maxReportedFailures {
				continue
			}
			reason := fmt.Sprintf("index=%s status=%d", state.Index, state.Status)
			if len(state.Error) > 0 {
				reason += " error=" + truncateUTF8(string(state.Error), maxErrorPayloadBytes)
			}
			sample = append(sample, reason)
		}
	}
	return count, sample
}

// truncateUTF8 shortens s to at most maxBytes, backing up to a rune boundary so
// it never emits invalid UTF-8, and appends an ellipsis when it cuts.
func truncateUTF8(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	for maxBytes > 0 && !utf8.RuneStart(s[maxBytes]) {
		maxBytes--
	}
	return s[:maxBytes] + "…"
}
