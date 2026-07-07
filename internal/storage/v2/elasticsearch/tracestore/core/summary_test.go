// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/olivere/elastic/v7"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/indices"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/snapshottest"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core/dbmodel"
)

const summaryAggregationJSON = `{
  "buckets": [
    {
      "key": "00000000000000000000000000000001",
      "doc_count": 3,
      "min_start": {"value": 1000000},
      "max_start": {"value": 1500000},
      "max_end": {"value": 2000000},
      "error_count": {"doc_count": 1},
      "services": {"buckets": [
        {"key": "svcB", "doc_count": 1, "service_errors": {"doc_count": 0}},
        {"key": "svcA", "doc_count": 2, "service_errors": {"doc_count": 1}}
      ]},
      "root_span": {"doc_count": 1, "root_hit": {"hits": {"hits": [
        {"_source": {"operationName": "root-op", "process": {"serviceName": "svcA"}}}
      ]}}}
    }
  ]
}`

// Phase 1 (findTraceIDsFromQuery) reads this "traceIDs" aggregation; phase 2 reads
// "trace_summaries". A single mocked search result carries both so the two-phase
// FindTraceSummaries can run end to end.
const traceIDsAggregationJSON = `{
  "buckets": [
    {"key": "00000000000000000000000000000001", "doc_count": 3, "startTime": {"value": 1500000}}
  ]
}`

func summaryResult(summaryJSON string) *elastic.SearchResult {
	return &elastic.SearchResult{Aggregations: elastic.Aggregations{
		traceIDAggregation: []byte(traceIDsAggregationJSON),
		"trace_summaries":  []byte(summaryJSON),
	}}
}

func mockSummarySearchServiceObj(r *spanReaderTest) *mocks.SearchService {
	searchService := &mocks.SearchService{}
	searchService.On("Query", mock.Anything).Return(searchService)
	searchService.On("IgnoreUnavailable", mock.AnythingOfType("bool")).Return(searchService)
	searchService.On("Size", mock.AnythingOfType("int")).Return(searchService)
	searchService.On("Aggregation", mock.AnythingOfType("string"), mock.Anything).Return(searchService)
	r.client.On("Search", mock.AnythingOfType("[]string")).Return(searchService)
	return searchService
}

func mockSummarySearchService(r *spanReaderTest) *mock.Call {
	return mockSummarySearchServiceObj(r).On("Do", mock.Anything)
}

// traceIDsResult is a phase-1-only search result: it carries just the traceIDs
// aggregation, so findTraceIDsFromQuery succeeds and FindTraceSummaries proceeds
// to phase 2. Pairing it (via .Once()) with a second Do return lets tests drive
// the phase-2 branches that phase 1 would otherwise short-circuit.
func traceIDsResult() *elastic.SearchResult {
	return &elastic.SearchResult{Aggregations: elastic.Aggregations{
		traceIDAggregation: []byte(traceIDsAggregationJSON),
	}}
}

func validSummaryQuery() dbmodel.TraceQueryParameters {
	return dbmodel.TraceQueryParameters{
		ServiceName:  serviceName,
		StartTimeMin: time.Now().Add(-time.Hour),
		StartTimeMax: time.Now(),
		SearchDepth:  10,
	}
}

func TestSpanReader_FindTraceSummaries_IndexWindowMatchesMaxTraceDuration(t *testing.T) {
	// Regression test for the phase-2 index selection window. The summary aggregation
	// must search the same ±maxTraceDuration window of indices that multiRead uses,
	// not a narrower ±1h window. With daily indices, a trace matched in the middle of
	// a day can have spans in the adjacent day within maxTraceDuration; if those
	// indices are not searched the summary (SpanCount, services, errors, duration) is
	// partial. The withSpanReader fixture uses daily indices and MaxTraceDuration=24h.
	withSpanReader(t, func(r *spanReaderTest) {
		mockSummarySearchService(r).Return(summaryResult(summaryAggregationJSON), nil)

		const maxTraceDuration = 24 * time.Hour // matches the withSpanReader fixture
		day := time.Date(2019, 10, 10, 12, 0, 0, 0, time.UTC)
		query := dbmodel.TraceQueryParameters{
			ServiceName:  serviceName,
			StartTimeMin: day,
			StartTimeMax: day,
			SearchDepth:  10,
		}

		_, err := r.reader.FindTraceSummaries(context.Background(), query)
		require.NoError(t, err)

		rotation := indices.NewPeriodicRotation(config.SpanIndexName, "2006-01-02", maxTraceDuration)
		wideWindow := rotation.ReadTargets(day.Add(-maxTraceDuration), day.Add(maxTraceDuration))
		narrowWindow := rotation.ReadTargets(day.Add(-time.Hour), day.Add(time.Hour))
		// Guard: the fixture must actually distinguish the two windows, otherwise the
		// assertion below would pass even with the old ±1h padding.
		require.Greater(t, len(wideWindow), len(narrowWindow))
		r.client.AssertCalled(t, "Search", wideWindow)
	})
}

func TestSpanReader_FindTraceSummaries(t *testing.T) {
	withSpanReader(t, func(r *spanReaderTest) {
		mockSummarySearchService(r).
			Return(summaryResult(summaryAggregationJSON), nil)

		summaries, err := r.reader.FindTraceSummaries(context.Background(), validSummaryQuery())
		require.NoError(t, err)
		require.Len(t, summaries, 1)

		s := summaries[0]
		assert.Equal(t, dbmodel.TraceID("00000000000000000000000000000001"), s.TraceID)
		assert.Equal(t, 3, s.SpanCount)
		assert.Equal(t, 1, s.ErrorSpanCount)
		assert.Equal(t, uint64(1000000), s.MinStartTime)
		assert.Equal(t, uint64(2000000), s.MaxEndTime)
		assert.Equal(t, "svcA", s.RootServiceName)
		assert.Equal(t, "root-op", s.RootOperationName)

		require.Len(t, s.Services, 2)
		// Sorted by service name.
		assert.Equal(t, "svcA", s.Services[0].ServiceName)
		assert.Equal(t, 2, s.Services[0].SpanCount)
		assert.Equal(t, 1, s.Services[0].ErrorSpanCount)
		assert.Equal(t, "svcB", s.Services[1].ServiceName)
		assert.Equal(t, 1, s.Services[1].SpanCount)
		assert.Equal(t, 0, s.Services[1].ErrorSpanCount)
	})
}

func TestSpanReader_FindTraceSummaries_DefaultsSearchDepth(t *testing.T) {
	withSpanReader(t, func(r *spanReaderTest) {
		mockSummarySearchService(r).Return(summaryResult(summaryAggregationJSON), nil)
		// SearchDepth 0 must fall back to defaultSearchDepth rather than requesting
		// a zero-size terms aggregation.
		query := validSummaryQuery()
		query.SearchDepth = 0
		summaries, err := r.reader.FindTraceSummaries(context.Background(), query)
		require.NoError(t, err)
		require.Len(t, summaries, 1)
	})
}

// TestSpanReader_FindTraceSummaries_Phase2 drives the phase-2 failure branches
// that phase 1 short-circuits: phase 1 returns trace IDs (first Do), then phase 2
// (second Do) returns the failure under test.
func TestSpanReader_FindTraceSummaries_Phase2(t *testing.T) {
	missingSummary := &elastic.SearchResult{Aggregations: elastic.Aggregations{
		"other": []byte(`{"buckets": []}`),
	}}
	nonStringKey := &elastic.SearchResult{Aggregations: elastic.Aggregations{
		"trace_summaries": []byte(`{"buckets": [{"key": 123, "doc_count": 1}]}`),
	}}
	tests := []struct {
		name   string
		result *elastic.SearchResult
		err    error
	}{
		{name: "search error", err: errors.New("phase-2 search failed")},
		{name: "missing summaries aggregation", result: missingSummary},
		{name: "non-string trace ID key", result: nonStringKey},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withSpanReader(t, func(r *spanReaderTest) {
				ss := mockSummarySearchServiceObj(r)
				ss.On("Do", mock.Anything).Return(traceIDsResult(), nil).Once()
				ss.On("Do", mock.Anything).Return(tt.result, tt.err).Once()

				_, err := r.reader.FindTraceSummaries(context.Background(), validSummaryQuery())
				require.Error(t, err)
			})
		})
	}
}

func TestSpanReader_FindTraceSummaries_NilRootSource(t *testing.T) {
	withSpanReader(t, func(r *spanReaderTest) {
		// A root top-hit with no _source at all is a malformed response and must
		// surface as an error (distinct from a trace that simply has no root span).
		nilSource := `{
  "buckets": [
    {
      "key": "00000000000000000000000000000001",
      "doc_count": 1,
      "min_start": {"value": 1000000},
      "max_end": {"value": 2000000},
      "error_count": {"doc_count": 0},
      "services": {"buckets": []},
      "root_span": {"doc_count": 1, "root_hit": {"hits": {"hits": [{}]}}}
    }
  ]
}`
		mockSummarySearchService(r).Return(summaryResult(nilSource), nil)
		_, err := r.reader.FindTraceSummaries(context.Background(), validSummaryQuery())
		require.ErrorContains(t, err, "missing _source")
	})
}

func TestSpanReader_FindTraceSummaries_InvalidQuery(t *testing.T) {
	withSpanReader(t, func(r *spanReaderTest) {
		// Missing start/end time fails validation before any search is issued.
		_, err := r.reader.FindTraceSummaries(context.Background(), dbmodel.TraceQueryParameters{ServiceName: serviceName})
		require.Error(t, err)
	})
}

func TestSpanReader_FindTraceSummaries_SearchError(t *testing.T) {
	withSpanReader(t, func(r *spanReaderTest) {
		mockSummarySearchService(r).Return(nil, errors.New("search failed"))
		_, err := r.reader.FindTraceSummaries(context.Background(), validSummaryQuery())
		require.Error(t, err)
	})
}

// TestSpanReader_FindTraceSummaries_ScriptingDisabled verifies that a phase-2
// failure caused by inline scripting being disabled is reported as
// errors.ErrUnsupported, so the query service can fall back to client-side
// aggregation instead of failing the request.
func TestSpanReader_FindTraceSummaries_ScriptingDisabled(t *testing.T) {
	withSpanReader(t, func(r *spanReaderTest) {
		scriptErr := &elastic.Error{
			Status: 400,
			Details: &elastic.ErrorDetails{
				Type:   "search_phase_execution_exception",
				Reason: "all shards failed",
				RootCause: []*elastic.ErrorDetails{
					{Type: "illegal_argument_exception", Reason: "cannot execute [inline] scripts"},
				},
			},
		}
		ss := mockSummarySearchServiceObj(r)
		ss.On("Do", mock.Anything).Return(traceIDsResult(), nil).Once()
		ss.On("Do", mock.Anything).Return(nil, scriptErr).Once()

		_, err := r.reader.FindTraceSummaries(context.Background(), validSummaryQuery())
		require.ErrorIs(t, err, errors.ErrUnsupported)
	})
}

// TestSpanReader_FindTraceSummaries_PreMigrationRoot covers traces written before
// #8859, where no span stores a parentSpanID so every span matches the parentless
// filter; Elasticsearch's startTime-ascending sort then makes the earliest span
// the root. The aggregation reports multiple parentless spans (doc_count > 1) and
// the single top hit is that earliest span.
func TestSpanReader_FindTraceSummaries_PreMigrationRoot(t *testing.T) {
	withSpanReader(t, func(r *spanReaderTest) {
		summaryJSON := `{"buckets": [{
			"key": "00000000000000000000000000000001", "doc_count": 3,
			"min_start": {"value": 1000000}, "max_end": {"value": 2000000},
			"error_count": {"doc_count": 0}, "services": {"buckets": []},
			"root_span": {"doc_count": 3, "root_hit": {"hits": {"hits": [
				{"_source": {"operationName": "earliest-op", "process": {"serviceName": "svcEarliest"}}}
			]}}}
		}]}`
		mockSummarySearchService(r).Return(summaryResult(summaryJSON), nil)
		summaries, err := r.reader.FindTraceSummaries(context.Background(), validSummaryQuery())
		require.NoError(t, err)
		require.Len(t, summaries, 1)
		assert.Equal(t, "svcEarliest", summaries[0].RootServiceName)
		assert.Equal(t, "earliest-op", summaries[0].RootOperationName)
	})
}

func TestSpanReader_FindTraceSummaries_NoAggregations(t *testing.T) {
	withSpanReader(t, func(r *spanReaderTest) {
		mockSummarySearchService(r).Return(&elastic.SearchResult{Aggregations: nil}, nil)
		summaries, err := r.reader.FindTraceSummaries(context.Background(), validSummaryQuery())
		require.NoError(t, err)
		assert.Empty(t, summaries)
	})
}

func TestSpanReader_FindTraceSummaries_BadRootSource(t *testing.T) {
	withSpanReader(t, func(r *spanReaderTest) {
		// A malformed root-span _source must surface as an error, not be silently
		// turned into an empty root service/operation.
		badRoot := `{
  "buckets": [
    {
      "key": "00000000000000000000000000000001",
      "doc_count": 1,
      "min_start": {"value": 1000000},
      "max_end": {"value": 2000000},
      "error_count": {"doc_count": 0},
      "services": {"buckets": []},
      "root_span": {"doc_count": 1, "root_hit": {"hits": {"hits": [
        {"_source": "not-an-object"}
      ]}}}
    }
  ]
}`
		mockSummarySearchService(r).
			Return(summaryResult(badRoot), nil)
		_, err := r.reader.FindTraceSummaries(context.Background(), validSummaryQuery())
		require.Error(t, err)
	})
}

func TestSpanReader_FindTraceSummaries_NonStringServiceKey(t *testing.T) {
	withSpanReader(t, func(r *spanReaderTest) {
		// A non-string service-name bucket key is a malformed/schema-regressed response
		// and must fail fast, mirroring the trace-ID key handling, rather than producing
		// an empty-named service entry.
		badService := `{
  "buckets": [
    {
      "key": "00000000000000000000000000000001",
      "doc_count": 1,
      "min_start": {"value": 1000000},
      "max_end": {"value": 2000000},
      "error_count": {"doc_count": 0},
      "services": {"buckets": [{"key": 123, "doc_count": 1}]},
      "root_span": {"doc_count": 0, "root_hit": {"hits": {"hits": []}}}
    }
  ]
}`
		mockSummarySearchService(r).Return(summaryResult(badService), nil)
		_, err := r.reader.FindTraceSummaries(context.Background(), validSummaryQuery())
		require.ErrorContains(t, err, "non-string service name")
	})
}

func TestSpanReader_FindTraceSummaries_MissingBucketAggregation(t *testing.T) {
	withSpanReader(t, func(r *spanReaderTest) {
		aggs := map[string]json.RawMessage{"other": []byte(`{"buckets": []}`)}
		mockSummarySearchService(r).
			Return(&elastic.SearchResult{Aggregations: elastic.Aggregations(aggs)}, nil)
		_, err := r.reader.FindTraceSummaries(context.Background(), validSummaryQuery())
		require.ErrorIs(t, err, ErrUnableToFindTraceIDAggregation)
	})
}

func TestSpanReader_FindTraceSummaries_NoMatchingTraces(t *testing.T) {
	withSpanReader(t, func(r *spanReaderTest) {
		// Phase 1 finds no trace IDs, so no phase-2 aggregation runs.
		emptyIDs := `{"buckets": []}`
		result := &elastic.SearchResult{Aggregations: elastic.Aggregations{
			traceIDAggregation: []byte(emptyIDs),
		}}
		mockSummarySearchService(r).Return(result, nil)
		summaries, err := r.reader.FindTraceSummaries(context.Background(), validSummaryQuery())
		require.NoError(t, err)
		assert.Empty(t, summaries)
	})
}

// TestSpanReader_FindTraceSummaries_RootSpan covers root selection from the
// parentSpanID existence filter: the parentless span's service and operation are
// returned, and a trace with no parentless span yields an empty root.
func TestSpanReader_FindTraceSummaries_RootSpan(t *testing.T) {
	const traceID = "00000000000000000000000000000001"
	tests := []struct {
		name          string
		rootSpan      string
		wantService   string
		wantOperation string
	}{
		{
			name:          "parentless span yields its service and operation",
			rootSpan:      `{"doc_count": 1, "root_hit": {"hits": {"hits": [{"_source": {"operationName": "entry", "process": {"serviceName": "svcRoot"}}}]}}}`,
			wantService:   "svcRoot",
			wantOperation: "entry",
		},
		{
			name:          "no parentless span yields empty root",
			rootSpan:      `{"doc_count": 0, "root_hit": {"hits": {"hits": []}}}`,
			wantService:   "",
			wantOperation: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withSpanReader(t, func(r *spanReaderTest) {
				summaryJSON := fmt.Sprintf(`{"buckets": [{
					"key": "%s", "doc_count": 2,
					"min_start": {"value": 1000000}, "max_end": {"value": 2000000},
					"error_count": {"doc_count": 0}, "services": {"buckets": []},
					"root_span": %s
				}]}`, traceID, tt.rootSpan)
				mockSummarySearchService(r).Return(summaryResult(summaryJSON), nil)
				summaries, err := r.reader.FindTraceSummaries(context.Background(), validSummaryQuery())
				require.NoError(t, err)
				require.Len(t, summaries, 1)
				assert.Equal(t, tt.wantService, summaries[0].RootServiceName)
				assert.Equal(t, tt.wantOperation, summaries[0].RootOperationName)
			})
		})
	}
}

// TestSummaryRequestSnapshots freezes the wire format of the native trace-summaries
// aggregation. FindTraceSummaries runs two searches (phase 1 discovers matching
// trace IDs, phase 2 aggregates over all their spans); phase 1 is the same request
// already snapshot as find_trace_ids, so only the phase-2 request is captured here.
func TestSummaryRequestSnapshots(t *testing.T) {
	start := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	traceQuery := dbmodel.TraceQueryParameters{
		ServiceName:  "test-service",
		StartTimeMin: start,
		StartTimeMax: start.Add(time.Hour),
		SearchDepth:  20,
	}

	findTraceSummaries := map[es.BackendVersion]string{}
	for _, version := range es.AllVersions {
		rec := summaryRecorder()
		server := httptest.NewServer(rec)
		t.Cleanup(server.Close)
		client := newDataClient(t, server.URL, version)
		t.Cleanup(func() { _ = client.Close() })
		reader := newSnapshotReader(client)

		rec.Reset()
		_, err := reader.FindTraceSummaries(context.Background(), traceQuery)
		require.NoError(t, err)
		requests := rec.Requests()
		require.Len(t, requests, 2, "phase 1 (find trace IDs) + phase 2 (summaries)")
		// Snapshot only the phase-2 summaries search.
		findTraceSummaries[version] = snapshottest.Marshal(t, requests[1:])
	}

	snapshottest.AssertByVersion(t, "testdata/find_trace_summaries", findTraceSummaries)
}

// summaryRecorder returns one matching trace ID for the phase-1 find-trace-IDs
// search so phase 2 runs, and an empty summaries aggregation for phase 2.
func summaryRecorder() *snapshottest.Recorder {
	return snapshottest.NewRecorder(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		if bytes.Contains(body, []byte("trace_summaries")) {
			w.Write([]byte(`{"took":0,"hits":{"total":0,"hits":[]},"aggregations":{"trace_summaries":{"buckets":[]}}}`))
			return
		}
		w.Write([]byte(`{"took":0,"hits":{"total":0,"hits":[]},"aggregations":{"traceIDs":{"buckets":[{"key":"1234567890abcdef","doc_count":1}]}}}`))
	})
}
