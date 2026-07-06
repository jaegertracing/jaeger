// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package esclient

import (
	"net/http"
	"net/http/httptest"
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
}

func TestBulkIndexerFlushError(t *testing.T) {
	// A whole-request failure (non-2xx on _bulk) surfaces via esutil's OnError.
	_, url := bulkServer(t, func(w http.ResponseWriter) { w.WriteHeader(http.StatusInternalServerError) })
	core, logs := observer.New(zap.ErrorLevel)
	b, err := NewBulkIndexer(makeClient(t, url, "", "", es.ElasticV7), BulkIndexerConfig{}, metrics.NullFactory, zap.New(core))
	require.NoError(t, err)
	b.Add(BulkItem{Index: "idx", Body: map[string]any{"a": 1}})
	require.NoError(t, b.Close())
	assert.Positive(t, logs.FilterMessageSnippet("bulk indexer error").Len())
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
