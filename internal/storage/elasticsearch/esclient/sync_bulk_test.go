// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package esclient

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/metricstest"
	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
)

func newSyncWriter(t *testing.T, url string, maxBytes int, mf metrics.Factory, logger *zap.Logger) *SyncBulkWriter {
	return NewSyncBulkWriter(makeClient(t, url, "", "", es.ElasticV7), maxBytes, mf, logger)
}

// okBulkN answers with a successful _bulk response carrying exactly n item
// results, matching an n-document request (the sync writer requires the counts
// to agree).
func okBulkN(n int) func(http.ResponseWriter) {
	items := make([]string, n)
	for i := range items {
		items[i] = `{"index":{"status":201}}`
	}
	body := `{"errors":false,"items":[` + strings.Join(items, ",") + `]}`
	return func(w http.ResponseWriter) { w.Write([]byte(body)) }
}

func TestSyncBulkWriter_WritesNDJSON(t *testing.T) {
	rec, url := bulkServer(t, okBulkN(2))
	w := newSyncWriter(t, url, 0, metrics.NullFactory, zap.NewNop())

	err := w.Bulk(context.Background(), []BulkItem{
		{Index: "jaeger-span-000001", ID: "abc", Body: map[string]any{"traceID": "1"}},
		{Index: "jaeger.spans", OpType: es.WriteOpCreate, Body: map[string]any{"a": 1}},
	})
	require.NoError(t, err)

	require.Len(t, rec.Requests(), 1, "both items fit in one chunk")
	// The committed snapshot shows the exact _bulk wire format: an action line
	// (index/create with _index and optional _id) followed by its source line.
	rec.Assert(t, "testdata/sync_bulk_write")
}

func TestSyncBulkWriter_DefaultsMaxBytes(t *testing.T) {
	w := NewSyncBulkWriter(makeClient(t, "http://localhost:1", "", ""), -1, metrics.NullFactory, zap.NewNop())
	assert.Equal(t, defaultSyncBulkMaxBytes, w.maxBytes)
}

func TestSyncBulkWriter_Empty(t *testing.T) {
	rec, url := bulkServer(t, okBulk)
	w := newSyncWriter(t, url, 0, metrics.NullFactory, zap.NewNop())
	require.NoError(t, w.Bulk(context.Background(), nil))
	assert.Empty(t, rec.Requests(), "an empty batch issues no request")
}

func TestSyncBulkWriter_ChunkSplitByMaxBytes(t *testing.T) {
	rec, url := bulkServer(t, okBulk)
	// One encoded item is well over 20 bytes, so a 20-byte cap forces each item
	// into its own chunk (the first item alone exceeds the cap but is still sent).
	w := newSyncWriter(t, url, 20, metrics.NullFactory, zap.NewNop())

	err := w.Bulk(context.Background(), []BulkItem{
		{Index: "idx", Body: map[string]any{"a": 1}},
		{Index: "idx", Body: map[string]any{"b": 2}},
	})
	require.NoError(t, err)
	require.Len(t, rec.Requests(), 2, "each oversized item is sent in its own chunk")
	// The snapshot is an array of two single-document _bulk requests, making the
	// chunk boundary the maxBytes cap forces visible.
	rec.Assert(t, "testdata/sync_bulk_chunk_split")
}

func TestSyncBulkWriter_ItemErrorPropagates(t *testing.T) {
	mf := metricstest.NewFactory(time.Second)
	defer mf.Stop()
	_, url := bulkServer(t, func(w http.ResponseWriter) {
		w.Write([]byte(`{"took":2,"errors":true,"items":[` +
			`{"index":{"_index":"idx","status":201}},` +
			`{"create":{"_index":"idx","_id":"dup-1","status":409,"error":{"type":"version_conflict_engine_exception","reason":"boom"}}}` +
			`]}`))
	})
	core, logs := observer.New(zap.ErrorLevel)
	w := newSyncWriter(t, url, 0, mf, zap.New(core))

	err := w.Bulk(context.Background(), []BulkItem{
		{Index: "idx", Body: map[string]any{"a": 1}},
		{Index: "idx", Body: map[string]any{"b": 2}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "1 of 2 bulk items rejected")
	assert.Contains(t, err.Error(), "id=dup-1")
	assert.Contains(t, err.Error(), "version_conflict_engine_exception")
	assert.Positive(t, logs.FilterMessageSnippet("rejected items").Len())
	mf.AssertCounterMetrics(
		t,
		metricstest.ExpectedMetric{Name: "bulk_index.inserts", Value: 1},
		metricstest.ExpectedMetric{Name: "bulk_index.errors", Value: 1},
	)
	// A parsed response records latency-ok even though one item was rejected: the
	// round-trip succeeded, so the rejection shows only in the errors counter.
	_, gauges := mf.Snapshot()
	assert.True(t, hasTimer(gauges, "bulk_index.latency-ok"), "item rejection still records latency-ok: %v", gauges)
	assert.False(t, hasTimer(gauges, "bulk_index.latency-err"), "item rejection must not record latency-err: %v", gauges)
}

func TestSyncBulkWriter_TransportErrorPropagates(t *testing.T) {
	mf := metricstest.NewFactory(time.Second)
	defer mf.Stop()
	_, url := bulkServer(t, func(w http.ResponseWriter) {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
	})
	w := newSyncWriter(t, url, 0, mf, zap.NewNop())

	err := w.Bulk(context.Background(), []BulkItem{{Index: "idx", Body: map[string]any{"a": 1}}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bulk request failed")
	// A whole-request failure durably indexes nothing: every item counts as error.
	mf.AssertCounterMetrics(t, metricstest.ExpectedMetric{Name: "bulk_index.errors", Value: 1})
	// The request itself failed, so it records latency-err, not latency-ok.
	_, gauges := mf.Snapshot()
	assert.True(t, hasTimer(gauges, "bulk_index.latency-err"), "a transport failure records latency-err: %v", gauges)
	assert.False(t, hasTimer(gauges, "bulk_index.latency-ok"), "a transport failure must not record latency-ok: %v", gauges)
}

func TestSyncBulkWriter_ItemCountMismatch(t *testing.T) {
	// A 200 whose item results don't match the request count (e.g. a proxy
	// truncated the body) can't be accounted per-item, so the whole chunk fails.
	_, url := bulkServer(t, okBulkN(1))
	w := newSyncWriter(t, url, 0, metrics.NullFactory, zap.NewNop())
	err := w.Bulk(context.Background(), []BulkItem{
		{Index: "idx", Body: map[string]any{"a": 1}},
		{Index: "idx", Body: map[string]any{"b": 2}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "malformed bulk response: 1 item results for 2 documents")
}

func TestSyncBulkWriter_CapsReportedFailures(t *testing.T) {
	const n = maxReportedFailures + 5
	items := make([]string, n)
	sent := make([]BulkItem, n)
	for i := range items {
		items[i] = fmt.Sprintf(`{"create":{"_index":"idx-%d","status":400,"error":{"type":"x","reason":"y"}}}`, i)
		sent[i] = BulkItem{Index: "idx", Body: map[string]any{"a": i}}
	}
	_, url := bulkServer(t, func(w http.ResponseWriter) {
		w.Write([]byte(`{"errors":true,"items":[` + strings.Join(items, ",") + `]}`))
	})
	w := newSyncWriter(t, url, 0, metrics.NullFactory, zap.NewNop())

	err := w.Bulk(context.Background(), sent)
	require.Error(t, err)
	// The true counts are always reported...
	assert.Contains(t, err.Error(), fmt.Sprintf("%d of %d bulk items rejected", n, n))
	// ...but only maxReportedFailures reasons are rendered, plus an "and N more".
	assert.Contains(t, err.Error(), fmt.Sprintf("…and %d more", n-maxReportedFailures))
	assert.Contains(t, err.Error(), "idx-0")
	assert.NotContains(t, err.Error(), fmt.Sprintf("idx-%d", maxReportedFailures), "reasons past the cap are omitted")
}

func TestSyncBulkWriter_TruncatesErrorPayload(t *testing.T) {
	huge := strings.Repeat("A", 5000)
	_, url := bulkServer(t, func(w http.ResponseWriter) {
		w.Write([]byte(`{"errors":true,"items":[{"create":{"_index":"idx","status":400,"error":{"reason":"` + huge + `"}}}]}`))
	})
	w := newSyncWriter(t, url, 0, metrics.NullFactory, zap.NewNop())

	err := w.Bulk(context.Background(), []BulkItem{{Index: "idx", Body: map[string]any{"a": 1}}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "…")
	assert.NotContains(t, err.Error(), huge, "the raw error payload must not appear in full")
	assert.Less(t, len(err.Error()), 600, "the rendered error must be bounded")
}

func TestSyncBulkWriter_ItemFailureDespiteErrorsFalse(t *testing.T) {
	// A malformed response with errors:false but a failing item must still be
	// surfaced — failures are derived from per-item status, not the top-level flag,
	// so a rejection can never be silently committed.
	_, url := bulkServer(t, func(w http.ResponseWriter) {
		w.Write([]byte(`{"errors":false,"items":[{"create":{"_index":"idx","status":400,"error":{"reason":"bad"}}}]}`))
	})
	w := newSyncWriter(t, url, 0, metrics.NullFactory, zap.NewNop())
	err := w.Bulk(context.Background(), []BulkItem{{Index: "idx", Body: map[string]any{"a": 1}}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "1 of 1 bulk items rejected")
}

func TestSyncBulkWriter_ErrorsFlagWithoutFailingItem(t *testing.T) {
	// A malformed response with errors:true but every item succeeded reports no
	// actual rejection, so the chunk is treated as durable.
	_, url := bulkServer(t, func(w http.ResponseWriter) {
		w.Write([]byte(`{"errors":true,"items":[{"index":{"_index":"idx","status":200}}]}`))
	})
	w := newSyncWriter(t, url, 0, metrics.NullFactory, zap.NewNop())
	require.NoError(t, w.Bulk(context.Background(), []BulkItem{{Index: "idx", Body: map[string]any{"a": 1}}}))
}

func TestSyncBulkWriter_ContextCancelledAborts(t *testing.T) {
	rec, url := bulkServer(t, okBulk)
	w := newSyncWriter(t, url, 0, metrics.NullFactory, zap.NewNop())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := w.Bulk(ctx, []BulkItem{
		{Index: "idx", Body: map[string]any{"a": 1}},
		{Index: "idx", Body: map[string]any{"b": 2}},
	})
	require.ErrorIs(t, err, context.Canceled)
	// Aborting before any round-trip yields the bare context error, not a chunk's
	// wrapped transport failure, and issues no request.
	assert.NotContains(t, err.Error(), "bulk request failed")
	assert.Empty(t, rec.Requests(), "no chunk is sent once the context is cancelled")
}

func TestSyncBulkWriter_MalformedItemResultFails(t *testing.T) {
	// A durable write must be positively acknowledged with a 2xx status. A result
	// that carries no explicit failure but also no acknowledgement — an empty item,
	// a missing status (parses to 0), a non-2xx-but-errorless status, or multiple
	// action entries — must fail the chunk, or Bulk would return nil for a document
	// the backend never confirmed storing.
	tests := []struct {
		name string
		item string
	}{
		{"empty item", `{}`},
		{"missing status", `{"index":{"_index":"idx"}}`},
		{"non-2xx without error", `{"index":{"_index":"idx","status":301}}`},
		{"multiple action entries", `{"index":{"status":201},"create":{"status":201}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, url := bulkServer(t, func(w http.ResponseWriter) {
				w.Write([]byte(`{"items":[` + tt.item + `]}`))
			})
			w := newSyncWriter(t, url, 0, metrics.NullFactory, zap.NewNop())
			err := w.Bulk(context.Background(), []BulkItem{{Index: "idx", Body: map[string]any{"a": 1}}})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "1 of 1 bulk items rejected")
		})
	}
}

func TestTruncateBytes(t *testing.T) {
	assert.Equal(t, "abc", truncateBytes([]byte("abc"), 10), "no truncation when within limit")
	assert.Equal(t, "ab…", truncateBytes([]byte("abcdef"), 2), "ASCII cut at the byte limit")
	// "x€y" is x(1) + €(E2 82 AC, 3) + y(1). Cutting at byte 2 lands mid-€, so it
	// backs up to the rune start (byte 1) rather than emit invalid UTF-8.
	assert.Equal(t, "x…", truncateBytes([]byte("x€y"), 2), "back up to a rune boundary")
	assert.True(t, utf8.ValidString(truncateBytes([]byte("€€€"), 2)), "never emits invalid UTF-8")
}

func TestSyncBulkWriter_UnparsableResponse(t *testing.T) {
	_, url := bulkServer(t, func(w http.ResponseWriter) {
		w.Write([]byte(`not json`))
	})
	w := newSyncWriter(t, url, 0, metrics.NullFactory, zap.NewNop())
	err := w.Bulk(context.Background(), []BulkItem{{Index: "idx", Body: map[string]any{"a": 1}}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse bulk response")
}

func TestSyncBulkWriter_EncodeError(t *testing.T) {
	mf := metricstest.NewFactory(time.Second)
	defer mf.Stop()
	rec, url := bulkServer(t, okBulk)
	w := newSyncWriter(t, url, 0, mf, zap.NewNop())

	err := w.Bulk(context.Background(), []BulkItem{{Index: "idx", Body: make(chan int)}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to encode bulk document")
	assert.Empty(t, rec.Requests(), "an unencodable document is never sent")
	mf.AssertCounterMetrics(t, metricstest.ExpectedMetric{Name: "bulk_index.errors", Value: 1})
}
