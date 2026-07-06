// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package esclient

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/metricstest"
	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/snapshottest"
)

// recordingServer is a running httptest server plus the Recorder capturing its
// requests and the URL to point a client at.
type recordingServer struct {
	*snapshottest.Recorder
	url string
}

// bulkTestServerRecorder starts a server that records requests and answers each
// with the response chosen by respond (keyed by 1-based call number).
func bulkTestServerRecorder(t *testing.T, respond func(call int, w http.ResponseWriter)) *recordingServer {
	var calls atomic.Int32
	rec := snapshottest.NewRecorder(func(w http.ResponseWriter, _ *http.Request) {
		respond(int(calls.Add(1)), w)
	})
	server := httptest.NewServer(rec)
	t.Cleanup(server.Close)
	return &recordingServer{Recorder: rec, url: server.URL}
}

func newTestIndexer(t *testing.T, url string, cfg BulkIndexerConfig, version es.BackendVersion, mf metrics.Factory) *BulkIndexer {
	return NewBulkIndexer(makeClient(t, url, "", "", version), cfg, mf, zap.NewNop())
}

func okBulk(w http.ResponseWriter) {
	w.Write([]byte(`{"errors":false,"items":[{"index":{"status":200}}]}`))
}

func TestBulkIndexerFlushOnClose(t *testing.T) {
	var calls atomic.Int32
	rec := snapshottest.NewRecorder(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		okBulk(w)
	})
	server := httptest.NewServer(rec)
	defer server.Close()

	b := newTestIndexer(t, server.URL, BulkIndexerConfig{}, es.ElasticV7, metrics.NullFactory)
	b.Add(BulkItem{Index: "jaeger-span-000001", Type: "span", Body: map[string]any{"traceID": "abc"}})
	require.NoError(t, b.Close())

	reqs := rec.Requests()
	require.Len(t, reqs, 1)
	assert.Equal(t, "/_bulk", reqs[0].Path)
	assert.Equal(t, http.MethodPost, reqs[0].Method)
	assert.Contains(t, string(reqs[0].Body), "jaeger-span-000001")
	assert.Contains(t, string(reqs[0].Body), `"traceID":"abc"`)
}

// TestBulkIndexerByteCap is the #2192 guard: no single _bulk request may exceed
// FlushBytes, and every document still lands.
func TestBulkIndexerByteCap(t *testing.T) {
	rec := snapshottest.NewRecorder(func(w http.ResponseWriter, _ *http.Request) { okBulk(w) })
	server := httptest.NewServer(rec)
	defer server.Close()

	const capBytes = 120
	b := newTestIndexer(t, server.URL, BulkIndexerConfig{FlushBytes: capBytes}, es.ElasticV7, metrics.NullFactory)
	const n = 6
	for i := range n {
		b.Add(BulkItem{Index: "jaeger-span-000001", Body: map[string]any{"i": i}})
	}
	require.NoError(t, b.Close())

	reqs := rec.Requests()
	require.Greater(t, len(reqs), 1, "byte cap should have forced multiple flushes")
	total := 0
	for _, r := range reqs {
		assert.LessOrEqual(t, len(r.Body), capBytes, "no request may exceed the byte cap")
		total += countBulkDocs(r.Body)
	}
	assert.Equal(t, n, total, "every document must be sent exactly once")
}

func TestBulkIndexerMaxActions(t *testing.T) {
	rec := snapshottest.NewRecorder(func(w http.ResponseWriter, _ *http.Request) { okBulk(w) })
	server := httptest.NewServer(rec)
	defer server.Close()

	b := newTestIndexer(t, server.URL, BulkIndexerConfig{MaxActions: 2}, es.ElasticV7, metrics.NullFactory)
	for i := range 5 {
		b.Add(BulkItem{Index: "idx", Body: map[string]any{"i": i}})
	}
	require.NoError(t, b.Close())

	reqs := rec.Requests()
	// 2 + 2 + 1 ⇒ three flushes.
	require.Len(t, reqs, 3)
	assert.Equal(t, 2, countBulkDocs(reqs[0].Body))
	assert.Equal(t, 2, countBulkDocs(reqs[1].Body))
	assert.Equal(t, 1, countBulkDocs(reqs[2].Body))
}

// TestBulkIndexerUnbounded verifies that zero limits (the query-service default)
// buffer every document into a single flush at Close, rather than flushing each.
func TestBulkIndexerUnbounded(t *testing.T) {
	rec := snapshottest.NewRecorder(func(w http.ResponseWriter, _ *http.Request) { okBulk(w) })
	server := httptest.NewServer(rec)
	defer server.Close()

	b := newTestIndexer(t, server.URL, BulkIndexerConfig{FlushBytes: -1}, es.ElasticV7, metrics.NullFactory)
	for i := range 4 {
		b.Add(BulkItem{Index: "idx", Body: map[string]any{"i": i}})
	}
	require.Empty(t, rec.Requests(), "nothing should flush before Close when limits are disabled")
	require.NoError(t, b.Close())

	reqs := rec.Requests()
	require.Len(t, reqs, 1)
	assert.Equal(t, 4, countBulkDocs(reqs[0].Body))
}

// TestBulkIndexerFlushesAtExactMaxActions verifies the action-count trigger
// fires as soon as the batch reaches MaxActions, without waiting for a further
// Add or for Close (send is synchronous, so the request lands before Close).
func TestBulkIndexerFlushesAtExactMaxActions(t *testing.T) {
	rec := snapshottest.NewRecorder(func(w http.ResponseWriter, _ *http.Request) { okBulk(w) })
	server := httptest.NewServer(rec)
	defer server.Close()

	b := newTestIndexer(t, server.URL, BulkIndexerConfig{MaxActions: 2}, es.ElasticV7, metrics.NullFactory)
	t.Cleanup(func() { _ = b.Close() })
	b.Add(BulkItem{Index: "idx", Body: map[string]any{"a": 1}})
	assert.Empty(t, rec.Requests(), "one document should stay buffered")
	b.Add(BulkItem{Index: "idx", Body: map[string]any{"b": 2}})
	require.Len(t, rec.Requests(), 1, "reaching MaxActions flushes immediately, before Close")
	assert.Equal(t, 2, countBulkDocs(rec.Requests()[0].Body))
}

func TestBulkIndexerFlushInterval(t *testing.T) {
	rec := snapshottest.NewRecorder(func(w http.ResponseWriter, _ *http.Request) { okBulk(w) })
	server := httptest.NewServer(rec)
	defer server.Close()

	b := newTestIndexer(t, server.URL, BulkIndexerConfig{FlushInterval: 10 * time.Millisecond}, es.ElasticV7, metrics.NullFactory)
	t.Cleanup(func() { _ = b.Close() })
	b.Add(BulkItem{Index: "idx", Body: map[string]any{"a": 1}})

	require.Eventually(t, func() bool {
		return len(rec.Requests()) == 1
	}, time.Second, 5*time.Millisecond, "the interval timer should flush without Close")
}

func TestBulkIndexerPerItemRetry(t *testing.T) {
	rec := bulkTestServerRecorder(t, func(call int, w http.ResponseWriter) {
		if call == 1 {
			w.Write([]byte(`{"errors":true,"items":[{"index":{"status":429}}]}`))
			return
		}
		okBulk(w)
	})

	mf := metricstest.NewFactory(time.Second)
	defer mf.Stop()
	b := NewBulkIndexer(makeClient(t, rec.url, "", "", es.ElasticV7), BulkIndexerConfig{}, mf, zap.NewNop())
	b.Add(BulkItem{Index: "idx", Body: map[string]any{"a": 1}})
	require.NoError(t, b.Close())

	reqs := rec.Requests()
	require.Len(t, reqs, 2, "the 429 item should be retried once")
	assert.Contains(t, string(reqs[1].Body), `"a":1`)

	mf.AssertCounterMetrics(
		t,
		metricstest.ExpectedMetric{Name: "bulk_index.inserts", Value: 1},
		metricstest.ExpectedMetric{Name: "bulk_index.errors", Value: 0},
	)
}

func TestBulkIndexerWholeRequestRetry(t *testing.T) {
	rec := bulkTestServerRecorder(t, func(call int, w http.ResponseWriter) {
		if call == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		okBulk(w)
	})

	b := NewBulkIndexer(makeClient(t, rec.url, "", "", es.ElasticV7), BulkIndexerConfig{}, metrics.NullFactory, zap.NewNop())
	b.Add(BulkItem{Index: "idx", Body: map[string]any{"a": 1}})
	require.NoError(t, b.Close())

	require.Len(t, rec.Requests(), 2, "a 503 whole-request failure should be retried")
}

func TestBulkIndexerNonRetryableItemError(t *testing.T) {
	rec := bulkTestServerRecorder(t, func(_ int, w http.ResponseWriter) {
		w.Write([]byte(`{"errors":true,"items":[{"index":{"status":400,"error":{"type":"mapper_parsing_exception"}}}]}`))
	})

	mf := metricstest.NewFactory(time.Second)
	defer mf.Stop()
	b := NewBulkIndexer(makeClient(t, rec.url, "", "", es.ElasticV7), BulkIndexerConfig{}, mf, zap.NewNop())
	b.Add(BulkItem{Index: "idx", Body: map[string]any{"a": 1}})
	require.NoError(t, b.Close())

	require.Len(t, rec.Requests(), 1, "a 400 error must not be retried")
	mf.AssertCounterMetrics(
		t,
		metricstest.ExpectedMetric{Name: "bulk_index.attempts", Value: 1},
		metricstest.ExpectedMetric{Name: "bulk_index.inserts", Value: 0},
		metricstest.ExpectedMetric{Name: "bulk_index.errors", Value: 1},
	)
}

func TestBulkIndexerRetryExhaustion(t *testing.T) {
	rec := bulkTestServerRecorder(t, func(_ int, w http.ResponseWriter) {
		w.Write([]byte(`{"errors":true,"items":[{"index":{"status":429}}]}`))
	})

	b := NewBulkIndexer(makeClient(t, rec.url, "", "", es.ElasticV7), BulkIndexerConfig{}, metrics.NullFactory, zap.NewNop())
	b.Add(BulkItem{Index: "idx", Body: map[string]any{"a": 1}})
	require.NoError(t, b.Close())

	// initial send + maxBulkRetries.
	require.Len(t, rec.Requests(), 1+maxBulkRetries)
}

func TestBulkIndexerWholeRequestRetryExhaustion(t *testing.T) {
	rec := bulkTestServerRecorder(t, func(_ int, w http.ResponseWriter) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})

	b := NewBulkIndexer(makeClient(t, rec.url, "", "", es.ElasticV7), BulkIndexerConfig{}, metrics.NullFactory, zap.NewNop())
	b.Add(BulkItem{Index: "idx", Body: map[string]any{"a": 1}})
	require.NoError(t, b.Close())

	require.Len(t, rec.Requests(), 1+maxBulkRetries, "a persistently failing request stops after maxBulkRetries")
}

// TestBulkIndexerResponseItemMismatch guards the defensive handling when the
// server returns an item with no verb key, or more items than were sent.
func TestBulkIndexerResponseItemMismatch(t *testing.T) {
	rec := bulkTestServerRecorder(t, func(_ int, w http.ResponseWriter) {
		w.Write([]byte(`{"errors":true,"items":[{},{"index":{"status":200}}]}`))
	})

	b := NewBulkIndexer(makeClient(t, rec.url, "", "", es.ElasticV7), BulkIndexerConfig{}, metrics.NullFactory, zap.NewNop())
	b.Add(BulkItem{Index: "idx", Body: map[string]any{"a": 1}})
	require.NoError(t, b.Close())
	require.Len(t, rec.Requests(), 1)
}

func TestBulkIndexerMalformedResponse(t *testing.T) {
	rec := bulkTestServerRecorder(t, func(_ int, w http.ResponseWriter) {
		w.Write([]byte("not json"))
	})

	b := NewBulkIndexer(makeClient(t, rec.url, "", "", es.ElasticV7), BulkIndexerConfig{}, metrics.NullFactory, zap.NewNop())
	b.Add(BulkItem{Index: "idx", Body: map[string]any{"a": 1}})
	require.NoError(t, b.Close())
	require.Len(t, rec.Requests(), 1)
}

func TestBulkIndexerEncodeErrorDropped(t *testing.T) {
	rec := bulkTestServerRecorder(t, func(_ int, w http.ResponseWriter) { okBulk(w) })
	b := NewBulkIndexer(makeClient(t, rec.url, "", "", es.ElasticV7), BulkIndexerConfig{}, metrics.NullFactory, zap.NewNop())
	b.Add(BulkItem{Index: "idx", Body: make(chan int)}) // unmarshalable
	require.NoError(t, b.Close())
	assert.Empty(t, rec.Requests(), "an unencodable document is dropped, not sent")
}

func TestBulkIndexerAddAfterClose(t *testing.T) {
	rec := bulkTestServerRecorder(t, func(_ int, w http.ResponseWriter) { okBulk(w) })
	b := NewBulkIndexer(makeClient(t, rec.url, "", "", es.ElasticV7), BulkIndexerConfig{}, metrics.NullFactory, zap.NewNop())
	require.NoError(t, b.Close())
	require.NoError(t, b.Close(), "Close is idempotent")
	b.Add(BulkItem{Index: "idx", Body: map[string]any{"a": 1}})
	assert.Empty(t, rec.Requests(), "documents added after Close are dropped")
}

func TestBulkIndexerTypedBackendEmitsType(t *testing.T) {
	rec := bulkTestServerRecorder(t, func(_ int, w http.ResponseWriter) { okBulk(w) })
	b := NewBulkIndexer(makeClient(t, rec.url, "", "", es.ElasticV6), BulkIndexerConfig{}, metrics.NullFactory, zap.NewNop())
	b.Add(BulkItem{Index: "idx", Type: "span", Body: map[string]any{"a": 1}})
	require.NoError(t, b.Close())
	require.Len(t, rec.Requests(), 1)
	assert.Contains(t, string(rec.Requests()[0].Body), `"_type":"span"`)
}

// countBulkDocs counts documents in an NDJSON _bulk body (action+source line
// pairs).
func countBulkDocs(body []byte) int {
	lines := 0
	for _, b := range body {
		if b == '\n' {
			lines++
		}
	}
	return lines / 2
}
