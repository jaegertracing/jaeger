// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package esclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
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
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/snapshottest"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore/spanstoremetrics"
)

// bulkServer records requests and answers each with the response chosen by
// respond. It returns the Recorder (for captured requests) and the URL.
func bulkServer(t *testing.T, respond func(w http.ResponseWriter)) (*snapshottest.Recorder, string) {
	rec := snapshottest.NewRecorder(func(w http.ResponseWriter, _ *http.Request) { respond(w) })
	server := httptest.NewServer(rec)
	t.Cleanup(server.Close)
	return rec, server.URL
}

func okBulk(w http.ResponseWriter) {
	w.Write([]byte(`{"took":1,"errors":false,"items":[{"index":{"status":201}}]}`))
}

func TestBulkIndexerWritesNDJSON(t *testing.T) {
	rec, url := bulkServer(t, okBulk)
	b, err := NewBulkIndexer(makeClient(t, url, "", "", es.ElasticV7), BulkIndexerConfig{}, metrics.NullFactory, zap.NewNop())
	require.NoError(t, err)
	b.Add(BulkItem{Index: "jaeger-span-000001", ID: "abc", Body: map[string]any{"traceID": "1"}})
	require.NoError(t, b.Close())

	reqs := rec.Requests()
	require.Len(t, reqs, 1)
	assert.Equal(t, http.MethodPost, reqs[0].Method)
	assert.Contains(t, reqs[0].Path, "_bulk")
	body := string(reqs[0].Body)
	assert.Contains(t, body, `"_index":"jaeger-span-000001"`)
	assert.Contains(t, body, `"_id":"abc"`)
	assert.Contains(t, body, `"traceID":"1"`)
}

func TestBulkIndexerCreateOpType(t *testing.T) {
	rec, url := bulkServer(t, okBulk)
	b, err := NewBulkIndexer(makeClient(t, url, "", "", es.ElasticV7), BulkIndexerConfig{}, metrics.NullFactory, zap.NewNop())
	require.NoError(t, err)
	b.Add(BulkItem{Index: "jaeger.spans", OpType: es.WriteOpCreate, Body: map[string]any{"a": 1}})
	require.NoError(t, b.Close())
	require.Len(t, rec.Requests(), 1)
	assert.Contains(t, string(rec.Requests()[0].Body), `"create":`)
}

func TestBulkIndexerEncodeErrorDropped(t *testing.T) {
	rec, url := bulkServer(t, okBulk)
	core, logs := observer.New(zap.ErrorLevel)
	mf := metricstest.NewFactory(time.Second)
	defer mf.Stop()
	b, err := NewBulkIndexer(makeClient(t, url, "", "", es.ElasticV7), BulkIndexerConfig{}, mf, zap.New(core))
	require.NoError(t, err)
	b.Add(BulkItem{Index: "idx", Body: make(chan int)}) // unmarshalable
	require.NoError(t, b.Close())
	assert.Empty(t, rec.Requests(), "an unencodable document is never sent")
	assert.Positive(t, logs.FilterMessageSnippet("failed to encode").Len())
	mf.AssertCounterMetrics(t, metricstest.ExpectedMetric{Name: "bulk_index.errors", Value: 1})
}

func TestBulkIndexerSuccessMetrics(t *testing.T) {
	_, url := bulkServer(t, okBulk)
	mf := metricstest.NewFactory(time.Second)
	defer mf.Stop()
	b, err := NewBulkIndexer(makeClient(t, url, "", "", es.ElasticV7), BulkIndexerConfig{}, mf, zap.NewNop())
	require.NoError(t, err)
	b.Add(BulkItem{Index: "idx", Body: map[string]any{"a": 1}})
	require.NoError(t, b.Close())
	mf.AssertCounterMetrics(
		t,
		metricstest.ExpectedMetric{Name: "bulk_index.inserts", Value: 1},
		metricstest.ExpectedMetric{Name: "bulk_index.errors", Value: 0},
	)
	_, gauges := mf.Snapshot()
	assert.True(t, hasTimer(gauges, "bulk_index.latency-ok"), "successful flush records latency-ok: %v", gauges)
	assert.False(t, hasTimer(gauges, "bulk_index.latency-err"), "successful flush must not record latency-err: %v", gauges)
}

func TestBulkIndexerFlushError(t *testing.T) {
	// A whole-request failure (non-2xx on _bulk) surfaces via esutil's OnError and,
	// because esutil also fans the flush error out to every item's OnFailure, still
	// increments attempts+errors (not silently swallowed) and records the flush
	// latency under latency-err rather than latency-ok.
	_, url := bulkServer(t, func(w http.ResponseWriter) { w.WriteHeader(http.StatusInternalServerError) })
	mf := metricstest.NewFactory(time.Second)
	defer mf.Stop()
	core, logs := observer.New(zap.ErrorLevel)
	b, err := NewBulkIndexer(makeClient(t, url, "", "", es.ElasticV7), BulkIndexerConfig{}, mf, zap.New(core))
	require.NoError(t, err)
	b.Add(BulkItem{Index: "idx", Body: map[string]any{"a": 1}})
	require.NoError(t, b.Close())
	assert.Positive(t, logs.FilterMessageSnippet("bulk indexer error").Len())
	mf.AssertCounterMetrics(
		t,
		metricstest.ExpectedMetric{Name: "bulk_index.inserts", Value: 0},
		metricstest.ExpectedMetric{Name: "bulk_index.errors", Value: 1},
	)
	_, gauges := mf.Snapshot()
	assert.True(t, hasTimer(gauges, "bulk_index.latency-err"), "failed flush records latency-err: %v", gauges)
	assert.False(t, hasTimer(gauges, "bulk_index.latency-ok"), "failed flush must not record latency-ok: %v", gauges)
}

// hasTimer reports whether the snapshot holds any percentile gauge for the named
// timer (metricstest emits timers as <name>.P50/P90/... gauges).
func hasTimer(gauges map[string]int64, name string) bool {
	for k := range gauges {
		if strings.HasPrefix(k, name+".") {
			return true
		}
	}
	return false
}

func TestBulkIndexerHonorsWorkerCount(t *testing.T) {
	_, url := bulkServer(t, okBulk)
	b, err := NewBulkIndexer(makeClient(t, url, "", "", es.ElasticV7), BulkIndexerConfig{Workers: 3}, metrics.NullFactory, zap.NewNop())
	require.NoError(t, err)
	b.Add(BulkItem{Index: "idx", Body: map[string]any{"a": 1}})
	require.NoError(t, b.Close())
}

// TestBulkIndexerFlushCallbacksWithoutState drives the flush callbacks with a
// bare context (no flush state stamped by onFlushStart) to exercise the
// defensive branches esutil never triggers itself: onFlushError just logs and
// onFlushEnd records no latency.
func TestBulkIndexerFlushCallbacksWithoutState(t *testing.T) {
	mf := metricstest.NewFactory(time.Second)
	defer mf.Stop()
	core, logs := observer.New(zap.ErrorLevel)
	b := &BulkIndexer{
		metrics: spanstoremetrics.NewWriter(mf, "bulk_index"),
		logger:  zap.New(core),
	}

	b.onFlushError(context.Background(), errors.New("boom"))
	b.onFlushEnd(context.Background())

	assert.Positive(t, logs.FilterMessageSnippet("bulk indexer error").Len())
	_, gauges := mf.Snapshot()
	assert.False(t, hasTimer(gauges, "bulk_index.latency-ok"))
	assert.False(t, hasTimer(gauges, "bulk_index.latency-err"))
}

// TestBulkIndexerRecordDropped covers the drop path shared by the encode-failure
// and enqueue-failure branches (the latter is unreachable through esutil.Add with
// our inputs, so it is exercised directly here).
func TestBulkIndexerRecordDropped(t *testing.T) {
	mf := metricstest.NewFactory(time.Second)
	defer mf.Stop()
	core, logs := observer.New(zap.ErrorLevel)
	b := &BulkIndexer{
		metrics: spanstoremetrics.NewWriter(mf, "bulk_index"),
		logger:  zap.New(core),
	}

	b.recordDropped("idx", "failed to enqueue bulk document", errors.New("queue full"))

	mf.AssertCounterMetrics(
		t,
		metricstest.ExpectedMetric{Name: "bulk_index.attempts", Value: 1},
		metricstest.ExpectedMetric{Name: "bulk_index.errors", Value: 1},
	)
	assert.Positive(t, logs.FilterMessageSnippet("failed to enqueue").Len())
}

func TestBulkIndexerFailureMetrics(t *testing.T) {
	_, url := bulkServer(t, func(w http.ResponseWriter) {
		w.Write([]byte(`{"took":1,"errors":true,"items":[{"index":{"status":400,"error":{"type":"mapper_parsing_exception"}}}]}`))
	})
	mf := metricstest.NewFactory(time.Second)
	defer mf.Stop()
	core, logs := observer.New(zap.ErrorLevel)
	b, err := NewBulkIndexer(makeClient(t, url, "", "", es.ElasticV7), BulkIndexerConfig{}, mf, zap.New(core))
	require.NoError(t, err)
	b.Add(BulkItem{Index: "idx", Body: map[string]any{"a": 1}})
	require.NoError(t, b.Close())
	mf.AssertCounterMetrics(
		t,
		metricstest.ExpectedMetric{Name: "bulk_index.inserts", Value: 0},
		metricstest.ExpectedMetric{Name: "bulk_index.errors", Value: 1},
	)
	assert.Positive(t, logs.FilterMessageSnippet("part of bulk request failed").Len())
}
