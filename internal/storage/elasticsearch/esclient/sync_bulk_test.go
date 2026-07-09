// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package esclient

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

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

	reqs := rec.Requests()
	require.Len(t, reqs, 1, "both items fit in one chunk")
	assert.Equal(t, http.MethodPost, reqs[0].Method)
	assert.Contains(t, reqs[0].Path, "_bulk")
	body := string(reqs[0].Body)
	// Map keys serialize in sorted order, so _id precedes _index in the action line.
	assert.Contains(t, body, `{"index":{"_id":"abc","_index":"jaeger-span-000001"}}`)
	assert.Contains(t, body, `"traceID":"1"`)
	assert.Contains(t, body, `{"create":{"_index":"jaeger.spans"}}`)
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
	assert.Len(t, rec.Requests(), 2, "each oversized item is sent in its own chunk")
}

func TestSyncBulkWriter_ItemErrorPropagates(t *testing.T) {
	mf := metricstest.NewFactory(time.Second)
	defer mf.Stop()
	_, url := bulkServer(t, func(w http.ResponseWriter) {
		w.Write([]byte(`{"took":2,"errors":true,"items":[` +
			`{"index":{"_index":"idx","status":201}},` +
			`{"create":{"_index":"idx","status":409,"error":{"type":"version_conflict_engine_exception","reason":"boom"}}}` +
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
	assert.Contains(t, err.Error(), "version_conflict_engine_exception")
	assert.Positive(t, logs.FilterMessageSnippet("rejected items").Len())
	mf.AssertCounterMetrics(
		t,
		metricstest.ExpectedMetric{Name: "bulk_index.inserts", Value: 1},
		metricstest.ExpectedMetric{Name: "bulk_index.errors", Value: 1},
	)
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
