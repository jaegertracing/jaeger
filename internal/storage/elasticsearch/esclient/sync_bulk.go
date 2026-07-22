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
// transport. Bulk blocks until the backend responds and returns an error iff any
// document in the batch was not durably stored. That single error is a per-batch
// durability verdict, derived from inspecting each item's status in the response —
// not a structured per-item list; the caller (WriteTraces) only needs pass/fail.
//
// It is a peer of the async BulkIndexer over the same Client, not a method on it,
// because esutil's blocking Flush cannot express that verdict. Flush returns nil
// for a normal 200 _bulk response even when individual items were rejected — a
// mapping rejection or version conflict is transport-level success — and delivers
// each item's real outcome only to the OnSuccess/OnFailure callbacks passed on Add.
// Its return value reports transport health, not per-item durability, so it is not
// the same single error Bulk returns despite both being a lone error value.
// Reconstructing the verdict from those callbacks is further complicated by esutil's
// shared, worker-pooled buffer, which has no per-call boundary (a Flush drains every
// concurrent caller's queued items). One direct _bulk round-trip that reads the
// response is simpler and yields exactly the verdict the synchronous mode needs.
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

	var (
		errs      []error
		chunk     []byte
		chunkLen  int
		succeeded int
	)
	flush := func() {
		if chunkLen == 0 {
			return
		}
		ok, err := w.sendChunk(ctx, chunk, chunkLen)
		succeeded += ok
		if err != nil {
			errs = append(errs, err)
		}
		chunk, chunkLen = nil, 0
	}
	// Encode each document as we pack it, rather than pre-encoding the whole batch
	// into a slice — that would hold the entire NDJSON payload in memory on top of
	// the active chunk (a full extra copy), risking OOM for large retried batches.
	// Only the current chunk (bounded by maxBytes) plus one transient document are
	// live at a time.
	for i := range items {
		if err := ctx.Err(); err != nil {
			// The caller's deadline or cancellation fired. Stop encoding and drop the
			// pending chunk rather than issue more round-trips that can only fail, and
			// return the single context error instead of a pile of transport failures.
			errs = append(errs, err)
			chunk, chunkLen = nil, 0
			break
		}
		blob, err := encodeBulkItem(items[i])
		if err != nil {
			// A span/service document is JSON-encodable, but a caller could pass a
			// Body json.Marshal rejects (an unsupported or cyclic value). Drop the
			// pending chunk and fail; any chunks already flushed above are durable
			// (unavoidable once split — tolerated under at-least-once).
			errs = append(errs, fmt.Errorf("failed to encode bulk document for index %q: %w", items[i].Index, err))
			chunk, chunkLen = nil, 0
			break
		}
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
	// The HTTP round-trip and response parsing succeeded, so record latency-ok even
	// when some items are rejected below. This matches the async BulkIndexer, whose
	// latency-err covers only a whole-request (transport/non-2xx) failure while
	// per-item rejections are reflected through the errors counter alone; latency-err
	// therefore keeps its meaning of "the request failed", not "some items failed".
	success = true
	// Derive failures from the per-item statuses, not the top-level `errors` flag:
	// a malformed or proxied response could report errors:false while an item still
	// carries a failing status, and silently succeeding there would advance the
	// Kafka offset over lost data — exactly what this synchronous writer prevents.
	failed, sample := resp.failures()
	if failed == 0 {
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
	var marshalErrors []error
	metaLine, err := json.Marshal(meta)
	marshalErrors = append(marshalErrors, err)
	source, err := json.Marshal(item.Body)
	marshalErrors = append(marshalErrors, err)
	if err := errors.Join(marshalErrors...); err != nil {
		return nil, err
	}
	var blob []byte
	blob = append(blob, metaLine...)
	blob = append(blob, '\n')
	blob = append(blob, source...)
	blob = append(blob, '\n')
	return blob, nil
}

// bulkResponse is the subset of the _bulk response we act on: each item's
// action-keyed result (status + optional error). The top-level `errors` flag is
// intentionally not parsed — failures are derived from the per-item statuses, so
// a malformed response that omits or negates the flag cannot hide a rejection.
type bulkResponse struct {
	Items []map[string]bulkItemState `json:"items"`
}

type bulkItemState struct {
	Index  string          `json:"_index"`
	ID     string          `json:"_id"`
	Status int             `json:"status"`
	Error  json.RawMessage `json:"error"`
}

// failures returns the number of items that are not a confirmed durable write and
// a bounded, human-readable sample of their reasons — at most maxReportedFailures
// entries, each error payload truncated to maxErrorPayloadBytes — so the returned
// error stays small even when an entire large batch is rejected.
//
// Durability is positively confirmed, not merely assumed from the absence of an
// error: an item counts as durable only when it has exactly one action result
// with a 2xx status and no error object. Anything else fails the chunk — a non-2xx
// status, a present error, an empty item ({}), a missing status (which parses to
// 0), or multiple action entries — because none of those is an acknowledgement, and
// treating them as success would let Bulk return nil without the backend having
// stored the document.
func (r bulkResponse) failures() (count int, sample []string) {
	for _, item := range r.Items {
		state, durable := itemResult(item)
		if durable {
			continue
		}
		count++
		if len(sample) >= maxReportedFailures {
			continue
		}
		sample = append(sample, rejectionReason(item, state))
	}
	return count, sample
}

// itemResult returns a bulk item's single action result and whether the item is a
// confirmed durable write (exactly one action entry, 2xx status, no error). A
// malformed item — zero or multiple entries — is never durable, and its returned
// state is the zero value.
func itemResult(item map[string]bulkItemState) (bulkItemState, bool) {
	if len(item) != 1 {
		return bulkItemState{}, false
	}
	var state bulkItemState
	for _, s := range item { // exactly one entry
		state = s
	}
	durable := state.Status >= http.StatusOK && state.Status < http.StatusMultipleChoices && len(state.Error) == 0
	return state, durable
}

// rejectionReason renders one rejected item for the error sample. A malformed item
// (not exactly one action entry) is reported as such; otherwise the reason carries
// the index, status, optional _id, and the truncated backend error.
func rejectionReason(item map[string]bulkItemState, state bulkItemState) string {
	if len(item) != 1 {
		return fmt.Sprintf("malformed item result: expected 1 action entry, got %d", len(item))
	}
	reason := fmt.Sprintf("index=%s status=%d", state.Index, state.Status)
	if state.ID != "" {
		reason += " id=" + state.ID
	}
	if len(state.Error) > 0 {
		reason += " error=" + truncateBytes(state.Error, maxErrorPayloadBytes)
	}
	return reason
}

// truncateBytes returns b as a string of at most maxBytes bytes, backing up to a
// UTF-8 rune boundary so it never emits invalid UTF-8, and appending an ellipsis
// when it cuts. It slices before converting to string, so a large payload is
// never copied in full — the transient allocation is bounded to maxBytes.
func truncateBytes(b []byte, maxBytes int) string {
	if len(b) <= maxBytes {
		return string(b)
	}
	for maxBytes > 0 && !utf8.RuneStart(b[maxBytes]) {
		maxBytes--
	}
	return string(b[:maxBytes]) + "…"
}
