// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/esclient"
	esclientmocks "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/esclient/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/indices"
	esquery "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/query"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/snapshottest"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core/dbmodel"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

const (
	defaultMaxDocCount = 10_000
	testingTraceId     = "testing-id"
)

var exampleESSpan = []byte(
	`{
	   "traceID": "1",
	   "parentSpanID": "2",
	   "spanID": "3",
	   "flags": 0,
	   "operationName": "op",
	   "references": [],
	   "startTime": 812965625,
	   "duration": 3290114992,
	   "tags": [
	      {
		 "key": "tag",
		 "value": "1965806585",
		 "type": "int64"
	      }
	   ],
	   "logs": [
	      {
		 "timestamp": 812966073,
		 "fields": [
		    {
		       "key": "logtag",
		       "value": "helloworld",
		       "type": "string"
		    }
		 ]
	      }
	   ],
	   "process": {
	      "serviceName": "serv",
	      "tags": [
		 {
		    "key": "processtag",
		    "value": "false",
		    "type": "bool"
		 }
	      ]
	   }
	}`,
)

type spanReaderTest struct {
	searcher    *esclientmocks.Searcher
	logger      *zap.Logger
	logBuffer   *testutils.Buffer
	traceBuffer *tracetest.InMemoryExporter
	reader      *SpanReader
}

func tracerProvider(t *testing.T) (trace.TracerProvider, *tracetest.InMemoryExporter, func()) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSyncer(exporter),
	)
	closer := func() {
		require.NoError(t, tp.Shutdown(context.Background()))
	}
	return tp, exporter, closer
}

func withSpanReader(t *testing.T, fn func(r *spanReaderTest)) {
	searcher := esclientmocks.NewSearcher(t)
	tracer, exp, closer := tracerProvider(t)
	defer closer()
	logger, logBuffer := testutils.NewLogger()
	r := &spanReaderTest{
		searcher:    searcher,
		logger:      logger,
		logBuffer:   logBuffer,
		traceBuffer: exp,
		reader: NewSpanReader(SpanReaderParams{
			Searcher:          searcher,
			Logger:            zap.NewNop(),
			Tracer:            tracer.Tracer("test"),
			MaxSpanAge:        0,
			MaxTraceDuration:  24 * time.Hour,
			TagDotReplacement: "@",
			MaxDocCount:       defaultMaxDocCount,
			SpanRotation:      indices.NewPeriodicRotation(config.SpanIndexName, "2006-01-02", 24*time.Hour),
			ServiceRotation:   indices.NewPeriodicRotation(config.ServiceIndexName, "2006-01-02", 24*time.Hour),
		}),
	}
	fn(r)
}

func withArchiveSpanReader(t *testing.T, readAlias bool, readAliasSuffix string, fn func(r *spanReaderTest)) {
	searcher := esclientmocks.NewSearcher(t)
	tracer, exp, closer := tracerProvider(t)
	defer closer()
	logger, logBuffer := testutils.NewLogger()

	var spanRotation, serviceRotation indices.Rotation
	if readAlias {
		suffix := "read"
		if readAliasSuffix != "" {
			suffix = readAliasSuffix
		}
		spanRotation = indices.NewAliasedRotation(config.SpanIndexName+config.IndexSeparator+suffix, config.SpanIndexName+config.IndexSeparator+suffix)
		serviceRotation = indices.NewAliasedRotation(config.ServiceIndexName+config.IndexSeparator+suffix, config.ServiceIndexName+config.IndexSeparator+suffix)
	} else {
		spanRotation = indices.NewPeriodicRotation(config.SpanIndexName, "2006-01-02", 24*time.Hour)
		serviceRotation = indices.NewPeriodicRotation(config.ServiceIndexName, "2006-01-02", 24*time.Hour)
	}

	r := &spanReaderTest{
		searcher:    searcher,
		logger:      logger,
		logBuffer:   logBuffer,
		traceBuffer: exp,
		reader: NewSpanReader(SpanReaderParams{
			Searcher:          searcher,
			Logger:            zap.NewNop(),
			Tracer:            tracer.Tracer("test"),
			MaxSpanAge:        0,
			MaxTraceDuration:  24 * time.Hour,
			TagDotReplacement: "@",
			SpanRotation:      spanRotation,
			ServiceRotation:   serviceRotation,
		}),
	}
	fn(r)
}

func TestNewSpanReader(t *testing.T) {
	params := SpanReaderParams{
		MaxSpanAge: time.Hour * 72,
		Logger:     zaptest.NewLogger(t),
	}
	reader := NewSpanReader(params)
	require.NotNil(t, reader)
	assert.Equal(t, time.Hour*72, reader.maxSpanAge)
}

func TestSpanReaderRotations(t *testing.T) {
	date := time.Date(2019, 10, 10, 5, 0, 0, 0, time.UTC)

	logger, _ := testutils.NewLogger()
	tracer, _, closer := tracerProvider(t)
	defer closer()

	testCases := []struct {
		name            string
		spanRotation    indices.Rotation
		serviceRotation indices.Rotation
		expectedIndices []string
	}{
		{
			name:            "periodic rotations",
			spanRotation:    indices.NewPeriodicRotation(config.SpanIndexName, "2006-01-02-15", 24*time.Hour),
			serviceRotation: indices.NewPeriodicRotation(config.ServiceIndexName, "2006-01-02", 24*time.Hour),
			expectedIndices: []string{"jaeger-span-2019-10-10-05", "jaeger-service-2019-10-10"},
		},
		{
			name:            "aliased rotations",
			spanRotation:    indices.NewAliasedRotation("jaeger-span-write", "jaeger-span-read"),
			serviceRotation: indices.NewAliasedRotation("jaeger-service-write", "jaeger-service-read"),
			expectedIndices: []string{"jaeger-span-read", "jaeger-service-read"},
		},
		{
			name: "with remote clusters",
			spanRotation: indices.NewRemoteClusterRotation(
				indices.NewPeriodicRotation(config.SpanIndexName, "2006-01-02-15", 24*time.Hour),
				[]string{"cluster_one", "cluster_two"},
			),
			serviceRotation: indices.NewRemoteClusterRotation(
				indices.NewPeriodicRotation(config.ServiceIndexName, "2006-01-02", 24*time.Hour),
				[]string{"cluster_one", "cluster_two"},
			),
			expectedIndices: []string{
				"jaeger-span-2019-10-10-05",
				"cluster_one:jaeger-span-2019-10-10-05",
				"cluster_two:jaeger-span-2019-10-10-05",
				"jaeger-service-2019-10-10",
				"cluster_one:jaeger-service-2019-10-10",
				"cluster_two:jaeger-service-2019-10-10",
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := NewSpanReader(SpanReaderParams{
				Logger:          logger,
				Tracer:          tracer.Tracer("test"),
				SpanRotation:    tc.spanRotation,
				ServiceRotation: tc.serviceRotation,
			})
			actualSpan := r.spanRotation.ReadTargets(date, date)
			actualService := r.serviceRotation.ReadTargets(date, date)
			assert.Equal(t, tc.expectedIndices, append(actualSpan, actualService...))
		})
	}
}

func TestSpanReader_GetTrace(t *testing.T) {
	withSpanReader(t, func(r *spanReaderTest) {
		hits := []esclient.SearchHit{{Source: exampleESSpan}}
		mockMultiSearchService(r).Return([]esclient.SearchResponse{
			{Hits: esclient.HitsResult{Hits: hits}},
		}, nil)
		query := []dbmodel.TraceID{dbmodel.TraceID(testingTraceId)}
		trace, err := r.reader.GetTraces(context.Background(), query)
		require.NotEmpty(t, r.traceBuffer.GetSpans(), "Spans recorded")
		require.NoError(t, err)
		require.NotNil(t, trace)
		assert.Len(t, trace, 1)
		expectedSpans, err := r.reader.collectSpans(hits)
		require.NoError(t, err)

		require.Len(t, trace[0].Spans, 1)
		assert.Equal(t, trace[0].Spans[0], expectedSpans[0])
	})
}

func TestSpanReader_multiRead_followUp_query(t *testing.T) {
	withSpanReader(t, func(r *spanReaderTest) {
		traceID1 := dbmodel.TraceID(testingTraceId + "1")
		traceID2 := dbmodel.TraceID(testingTraceId + "2")
		date := time.Date(2019, 10, 10, 5, 0, 0, 0, time.UTC)
		spanID1 := dbmodel.Span{
			SpanID:    "0",
			TraceID:   traceID1,
			StartTime: model.TimeAsEpochMicroseconds(date),
			Tags:      []dbmodel.KeyValue{},
			Process: dbmodel.Process{
				Tags: []dbmodel.KeyValue{},
			},
		}
		spanBytesID1, err := json.Marshal(spanID1)
		require.NoError(t, err)
		spanID2 := dbmodel.Span{
			SpanID:    "0",
			TraceID:   traceID2,
			StartTime: model.TimeAsEpochMicroseconds(date),
			Tags:      []dbmodel.KeyValue{},
			Process: dbmodel.Process{
				Tags: []dbmodel.KeyValue{},
			},
		}
		spanBytesID2, err := json.Marshal(spanID2)
		require.NoError(t, err)

		// Round 1: two searches (one per trace ID). traceID1's response reports two
		// total hits but returns only one span, which triggers a follow-up search;
		// traceID2's response is complete.
		firstRound := []esclient.SearchResponse{
			{Hits: esclient.HitsResult{Total: esclient.TotalHits{Value: 2}, Hits: []esclient.SearchHit{{Source: spanBytesID1}}}},
			{Hits: esclient.HitsResult{Total: esclient.TotalHits{Value: 1}, Hits: []esclient.SearchHit{{Source: spanBytesID2}}}},
		}
		// Round 2: the single follow-up search for traceID1, now fully fetched.
		secondRound := []esclient.SearchResponse{
			{Hits: esclient.HitsResult{Total: esclient.TotalHits{Value: 2}, Hits: []esclient.SearchHit{{Source: spanBytesID1}}}},
		}

		// Every sub-request must page startTime-ascending, track total hits, and carry
		// the expected search_after cursor: the padded window start on round 1, and
		// the last span's startTime on the follow-up.
		initialCursor := model.TimeAsEpochMicroseconds(date.Add(-24 * time.Hour))
		paginates := func(req esclient.MultiSearchRequest, wantCursor uint64) bool {
			s := req.Search
			return len(s.Sort) == 1 &&
				s.Sort[0] == esclient.SortOrder{Field: startTimeField, Order: esquery.Ascending} &&
				s.TrackTotalHits &&
				len(s.SearchAfter) == 1 && s.SearchAfter[0] == any(wantCursor)
		}

		r.searcher.On("MultiSearch", mock.Anything, mock.MatchedBy(func(reqs []esclient.MultiSearchRequest) bool {
			return len(reqs) == 2 && paginates(reqs[0], initialCursor) && paginates(reqs[1], initialCursor)
		})).Return(firstRound, nil).Once()
		r.searcher.On("MultiSearch", mock.Anything, mock.MatchedBy(func(reqs []esclient.MultiSearchRequest) bool {
			return len(reqs) == 1 && paginates(reqs[0], spanID1.StartTime)
		})).Return(secondRound, nil).Once()

		traces, err := r.reader.multiRead(context.Background(), []dbmodel.TraceID{traceID1, traceID2}, date, date)
		require.NotEmpty(t, r.traceBuffer.GetSpans(), "Spans recorded")
		require.NoError(t, err)
		require.NotNil(t, traces)
		require.Len(t, traces, 2)

		for i, s := range []dbmodel.Span{spanID1, spanID2} {
			actual := traces[i].Spans[0]
			actualData, err := json.Marshal(actual)
			require.NoError(t, err)
			expectedData, err := json.Marshal(s)
			require.NoError(t, err)
			assert.Equal(t, string(expectedData), string(actualData))
		}
	})
}

func TestSpanReader_SearchAfter(t *testing.T) {
	withSpanReader(t, func(r *spanReaderTest) {
		var hits []esclient.SearchHit

		for range 10000 {
			hits = append(hits, esclient.SearchHit{Source: exampleESSpan})
		}

		resp := []esclient.SearchResponse{
			{Hits: esclient.HitsResult{Total: esclient.TotalHits{Value: 10040}, Hits: hits}},
		}
		mockMultiSearchService(r).Return(resp, nil).Times(2)

		query := []dbmodel.TraceID{dbmodel.TraceID("testing-id")}
		trace, err := r.reader.GetTraces(context.Background(), query)
		require.NotEmpty(t, r.traceBuffer.GetSpans(), "Spans recorded")
		require.NoError(t, err)
		require.NotNil(t, trace)
		assert.Len(t, trace, 1)
		expectedSpans, err := r.reader.collectSpans(hits)
		require.NoError(t, err)

		assert.Equal(t, trace[0].Spans[0], expectedSpans[0])
	})
}

func TestSpanReader_GetTraceQueryError(t *testing.T) {
	withSpanReader(t, func(r *spanReaderTest) {
		// An empty _msearch response set ends multiRead without producing traces.
		mockMultiSearchService(r).Return([]esclient.SearchResponse{}, nil)
		query := []dbmodel.TraceID{dbmodel.TraceID("testing-id")}
		trace, err := r.reader.GetTraces(context.Background(), query)
		require.NoError(t, err)
		require.NotEmpty(t, r.traceBuffer.GetSpans(), "Spans recorded")
		require.Empty(t, trace)
	})
}

func TestSpanReader_GetTraceNilHits(t *testing.T) {
	withSpanReader(t, func(r *spanReaderTest) {
		mockMultiSearchService(r).Return([]esclient.SearchResponse{
			{Hits: esclient.HitsResult{}},
		}, nil)

		query := []dbmodel.TraceID{dbmodel.TraceID(testingTraceId)}
		trace, err := r.reader.GetTraces(context.Background(), query)
		require.NoError(t, err)
		require.NotEmpty(t, r.traceBuffer.GetSpans(), "Spans recorded")
		require.Empty(t, trace)
	})
}

func TestSpanReader_GetTraceInvalidSpanError(t *testing.T) {
	withSpanReader(t, func(r *spanReaderTest) {
		data := []byte(`{"TraceID": "123"asdf fadsg}`)
		mockMultiSearchService(r).Return([]esclient.SearchResponse{
			{Hits: esclient.HitsResult{Hits: []esclient.SearchHit{{Source: data}}}},
		}, nil)

		query := []dbmodel.TraceID{dbmodel.TraceID(testingTraceId)}
		trace, err := r.reader.GetTraces(context.Background(), query)
		require.NotEmpty(t, r.traceBuffer.GetSpans(), "Spans recorded")
		require.Error(t, err, "invalid span")
		require.Nil(t, trace)
	})
}

func TestSpanReader_esJSONtoJSONSpanModel(t *testing.T) {
	withSpanReader(t, func(r *spanReaderTest) {
		esSpanRaw := esclient.SearchHit{Source: exampleESSpan}

		span, err := r.reader.unmarshalJSONSpan(esSpanRaw)
		require.NoError(t, err)

		var expectedSpan dbmodel.Span
		require.NoError(t, json.Unmarshal(exampleESSpan, &expectedSpan))
		assert.Equal(t, expectedSpan, span)
	})
}

func TestSpanReader_esJSONtoJSONSpanModelError(t *testing.T) {
	withSpanReader(t, func(r *spanReaderTest) {
		data := []byte(`{"TraceID": "123"asdf fadsg}`)
		esSpanRaw := esclient.SearchHit{Source: data}

		_, err := r.reader.unmarshalJSONSpan(esSpanRaw)
		require.Error(t, err)
	})
}

func TestSpanReaderFindIndices(t *testing.T) {
	today := time.Date(1995, time.April, 21, 4, 12, 19, 95, time.UTC)
	yesterday := today.AddDate(0, 0, -1)
	twoDaysAgo := today.AddDate(0, 0, -2)
	dateLayout := "2006-01-02"

	testCases := []struct {
		startTime time.Time
		endTime   time.Time
		expected  []string
	}{
		{
			startTime: today.Add(-time.Millisecond),
			endTime:   today,
			expected: []string{
				indices.IndexWithDate(config.SpanIndexName, dateLayout, today),
			},
		},
		{
			startTime: today.Add(-13 * time.Hour),
			endTime:   today,
			expected: []string{
				indices.IndexWithDate(config.SpanIndexName, dateLayout, today),
				indices.IndexWithDate(config.SpanIndexName, dateLayout, yesterday),
			},
		},
		{
			startTime: today.Add(-48 * time.Hour),
			endTime:   today,
			expected: []string{
				indices.IndexWithDate(config.SpanIndexName, dateLayout, today),
				indices.IndexWithDate(config.SpanIndexName, dateLayout, yesterday),
				indices.IndexWithDate(config.SpanIndexName, dateLayout, twoDaysAgo),
			},
		},
	}
	rotation := indices.NewPeriodicRotation(config.SpanIndexName, dateLayout, 24*time.Hour)
	for _, testCase := range testCases {
		actual := rotation.ReadTargets(testCase.startTime, testCase.endTime)
		assert.Equal(t, testCase.expected, actual)
	}
}

func TestSpanReaderIndexWithDate(t *testing.T) {
	withSpanReader(t, func(_ *spanReaderTest) {
		actual := indices.IndexWithDate(config.SpanIndexName, "2006-01-02", time.Date(1995, time.April, 21, 4, 21, 19, 95, time.UTC))
		assert.Equal(t, "jaeger-span-1995-04-21", actual)
	})
}

func testGet(typ string, t *testing.T) {
	goodResp := &esclient.SearchResponse{Aggregations: map[string]esclient.AggregationResult{
		typ: {Buckets: []esclient.AggregationBucket{{Key: "123", DocCount: 16}}},
	}}
	// Aggregations present but missing the requested bucket → "could not find".
	missingResp := &esclient.SearchResponse{Aggregations: map[string]esclient.AggregationResult{
		"other": {},
	}}

	testCases := []struct {
		caption        string
		resp           *esclient.SearchResponse
		searchError    error
		expectedError  func() string
		expectedOutput map[string]any
	}{
		{
			caption: typ + " full behavior",
			resp:    goodResp,
			expectedOutput: map[string]any{
				operationsAggregation: []dbmodel.Operation{{Name: "123"}},
				traceIDAggregation:    []dbmodel.TraceID{"123"},
				"default":             []string{"123"},
			},
			expectedError: func() string {
				return ""
			},
		},
		{
			caption:     typ + " search error",
			searchError: errors.New("Search failure"),
			expectedError: func() string {
				if typ == operationsAggregation {
					return "search operations failed: Search failure"
				}
				return "search services failed: Search failure"
			},
		},
		{
			caption: typ + " missing aggregation",
			resp:    missingResp,
			expectedError: func() string {
				return "could not find aggregation of " + typ
			},
		},
	}

	for _, tc := range testCases {
		testCase := tc
		t.Run(testCase.caption, func(t *testing.T) {
			withSpanReader(t, func(r *spanReaderTest) {
				mockSearchService(r).Return(testCase.resp, testCase.searchError)
				actual, err := returnSearchFunc(typ, r)
				if testCase.expectedError() != "" {
					require.EqualError(t, err, testCase.expectedError())
					assert.Nil(t, actual)
				} else if expectedOutput, ok := testCase.expectedOutput[typ]; ok {
					assert.Equal(t, expectedOutput, actual)
				} else {
					assert.Equal(t, testCase.expectedOutput["default"], actual)
				}
			})
		})
	}
}

func returnSearchFunc(typ string, r *spanReaderTest) (any, error) {
	switch typ {
	case servicesAggregation:
		return r.reader.GetServices(context.Background())
	case operationsAggregation:
		return r.reader.GetOperations(
			context.Background(),
			dbmodel.OperationQueryParameters{ServiceName: "someService"},
		)
	case traceIDAggregation:
		return r.reader.findTraceIDsFromQuery(context.Background(), dbmodel.TraceQueryParameters{})
	default:
		return nil, errors.New("Specify services, operations, traceIDs only")
	}
}

// TestSpanReader_findTraceIDsMultipleBuckets asserts every terms-aggregation
// bucket key is returned as a trace ID, in order.
func TestSpanReader_findTraceIDsMultipleBuckets(t *testing.T) {
	withSpanReader(t, func(r *spanReaderTest) {
		resp := &esclient.SearchResponse{Aggregations: map[string]esclient.AggregationResult{
			traceIDAggregation: {Buckets: []esclient.AggregationBucket{
				{Key: "hello"}, {Key: "world"}, {Key: "2"},
			}},
		}}
		mockSearchService(r).Return(resp, nil)

		actual, err := r.reader.findTraceIDsFromQuery(context.Background(), dbmodel.TraceQueryParameters{})
		require.NoError(t, err)
		assert.Equal(t, []dbmodel.TraceID{"hello", "world", "2"}, actual)
	})
}

func TestSpanReader_FindTraces(t *testing.T) {
	hits := []esclient.SearchHit{{Source: exampleESSpan}}

	withSpanReader(t, func(r *spanReaderTest) {
		// find trace IDs
		mockSearchService(r).Return(&esclient.SearchResponse{Aggregations: map[string]esclient.AggregationResult{
			traceIDAggregation: {Buckets: []esclient.AggregationBucket{{Key: "1"}, {Key: "2"}, {Key: "3"}}},
		}}, nil)
		// bulk read traces
		mockMultiSearchService(r).Return([]esclient.SearchResponse{
			{Hits: esclient.HitsResult{Hits: hits}},
			{Hits: esclient.HitsResult{Hits: hits}},
		}, nil)

		traceQuery := dbmodel.TraceQueryParameters{
			ServiceName: serviceName,
			Tags: map[string]string{
				"hello": "world",
			},
			StartTimeMin: time.Now().Add(-1 * time.Hour),
			StartTimeMax: time.Now(),
			SearchDepth:  1,
		}

		traces, err := r.reader.FindTraces(context.Background(), traceQuery)
		require.NotEmpty(t, r.traceBuffer.GetSpans(), "Spans recorded")
		require.NoError(t, err)
		assert.Len(t, traces, 1)

		trace := traces[0]
		expectedSpans, err := r.reader.collectSpans(hits)
		require.NoError(t, err)

		require.Len(t, trace.Spans, 2)
		assert.Equal(t, trace.Spans[0], expectedSpans[0])
	})
}

func TestSpanReader_FindTracesInvalidQuery(t *testing.T) {
	withSpanReader(t, func(r *spanReaderTest) {
		// Missing service name with tags fails validation before any search runs.
		traceQuery := dbmodel.TraceQueryParameters{
			ServiceName: "",
			Tags: map[string]string{
				"hello": "world",
			},
			StartTimeMin: time.Now().Add(-1 * time.Hour),
			StartTimeMax: time.Now(),
		}

		traces, err := r.reader.FindTraces(context.Background(), traceQuery)
		require.NotEmpty(t, r.traceBuffer.GetSpans(), "Spans recorded")
		require.Error(t, err)
		assert.Nil(t, traces)
	})
}

func TestSpanReader_FindTracesAggregationFailure(t *testing.T) {
	withSpanReader(t, func(r *spanReaderTest) {
		// Aggregations present but without the traceIDs bucket → aggregation error.
		mockSearchService(r).Return(&esclient.SearchResponse{
			Aggregations: map[string]esclient.AggregationResult{},
		}, nil)

		traceQuery := dbmodel.TraceQueryParameters{
			ServiceName: serviceName,
			Tags: map[string]string{
				"hello": "world",
			},
			StartTimeMin: time.Now().Add(-1 * time.Hour),
			StartTimeMax: time.Now(),
		}

		traces, err := r.reader.FindTraces(context.Background(), traceQuery)
		require.NotEmpty(t, r.traceBuffer.GetSpans(), "Spans recorded")
		require.Error(t, err)
		assert.Nil(t, traces)
	})
}

func TestSpanReader_FindTracesNoTraceIDs(t *testing.T) {
	withSpanReader(t, func(r *spanReaderTest) {
		mockSearchService(r).Return(&esclient.SearchResponse{Aggregations: map[string]esclient.AggregationResult{
			traceIDAggregation: {Buckets: []esclient.AggregationBucket{}},
		}}, nil)

		traceQuery := dbmodel.TraceQueryParameters{
			ServiceName: serviceName,
			Tags: map[string]string{
				"hello": "world",
			},
			StartTimeMin: time.Now().Add(-1 * time.Hour),
			StartTimeMax: time.Now(),
		}

		traces, err := r.reader.FindTraces(context.Background(), traceQuery)
		require.NotEmpty(t, r.traceBuffer.GetSpans(), "Spans recorded")
		require.NoError(t, err)
		assert.Empty(t, traces)
	})
}

func TestSpanReader_FindTracesReadTraceFailure(t *testing.T) {
	withSpanReader(t, func(r *spanReaderTest) {
		mockSearchService(r).Return(&esclient.SearchResponse{Aggregations: map[string]esclient.AggregationResult{
			traceIDAggregation: {Buckets: []esclient.AggregationBucket{{Key: "1"}, {Key: "2"}}},
		}}, nil)
		mockMultiSearchService(r).Return(nil, errors.New("read error"))

		traceQuery := dbmodel.TraceQueryParameters{
			ServiceName: serviceName,
			Tags: map[string]string{
				"hello": "world",
			},
			StartTimeMin: time.Now().Add(-1 * time.Hour),
			StartTimeMax: time.Now(),
		}

		traces, err := r.reader.FindTraces(context.Background(), traceQuery)
		require.NotEmpty(t, r.traceBuffer.GetSpans(), "Spans recorded")
		require.EqualError(t, err, "read error")
		assert.Empty(t, traces)
	})
}

func TestSpanReader_FindTracesSpanCollectionFailure(t *testing.T) {
	badSpan := []byte(`{"TraceID": "123"asjlgajdfhilqghi[adfvca} bad json`)
	badHits := []esclient.SearchHit{{Source: badSpan}}

	withSpanReader(t, func(r *spanReaderTest) {
		mockSearchService(r).Return(&esclient.SearchResponse{Aggregations: map[string]esclient.AggregationResult{
			traceIDAggregation: {Buckets: []esclient.AggregationBucket{{Key: "1"}, {Key: "2"}}},
		}}, nil)
		mockMultiSearchService(r).Return([]esclient.SearchResponse{
			{Hits: esclient.HitsResult{Hits: badHits}},
			{Hits: esclient.HitsResult{Hits: badHits}},
		}, nil)

		traceQuery := dbmodel.TraceQueryParameters{
			ServiceName: serviceName,
			Tags: map[string]string{
				"hello": "world",
			},
			StartTimeMin: time.Now().Add(-1 * time.Hour),
			StartTimeMax: time.Now(),
		}

		traces, err := r.reader.FindTraces(context.Background(), traceQuery)
		require.NotEmpty(t, r.traceBuffer.GetSpans(), "Spans recorded")
		require.Error(t, err)
		assert.Empty(t, traces)
	})
}

func TestFindTraceIDs(t *testing.T) {
	// Services/operations reads are covered by TestSpanReader_GetServices/
	// GetOperations; findTraceIDs runs over the esclient searcher exercised here.
	testGet(traceIDAggregation, t)
}

func TestReturnSearchFunc_DefaultCase(t *testing.T) {
	r := &spanReaderTest{}

	result, err := returnSearchFunc("unknownAggregationType", r)

	assert.Nil(t, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Specify services, operations, traceIDs only")
}

func mockMultiSearchService(r *spanReaderTest) *mock.Call {
	return r.searcher.On("MultiSearch", mock.Anything, mock.Anything)
}

func mockSearchService(r *spanReaderTest) *mock.Call {
	return r.searcher.On("Search", mock.Anything, mock.Anything, mock.Anything)
}

func TestTraceQueryParameterValidation(t *testing.T) {
	tqp := dbmodel.TraceQueryParameters{
		ServiceName: "",
		Tags: map[string]string{
			"hello": "world",
		},
	}
	err := validateQuery(tqp)
	require.EqualError(t, err, ErrServiceNameNotSet.Error())

	tqp.ServiceName = serviceName

	tqp.StartTimeMin = time.Time{} // time.Unix(0,0) doesn't work because timezones
	tqp.StartTimeMax = time.Time{}
	err = validateQuery(tqp)
	require.EqualError(t, err, ErrStartAndEndTimeNotSet.Error())

	tqp.StartTimeMin = time.Now()
	tqp.StartTimeMax = time.Now().Add(-1 * time.Hour)
	err = validateQuery(tqp)
	require.EqualError(t, err, ErrStartTimeMinGreaterThanMax.Error())

	tqp.StartTimeMin = time.Now().Add(-1 * time.Hour)
	tqp.StartTimeMax = time.Now()
	err = validateQuery(tqp)
	require.NoError(t, err)

	tqp.DurationMin = time.Hour
	tqp.DurationMax = time.Minute
	err = validateQuery(tqp)
	require.EqualError(t, err, ErrDurationMinGreaterThanMax.Error())
}

func TestSpanReader_buildTraceIDAggregation(t *testing.T) {
	expectedStr := `{ "terms":{
            "field":"traceID",
            "size":123,
            "order":{
               "startTime":"desc"
            }
         },
         "aggregations": {
            "startTime" : { "max": {"field": "startTime"}}
         }}`
	withSpanReader(t, func(r *spanReaderTest) {
		traceIDAggregation := r.reader.buildTraceIDAggregation(123)
		actual, err := traceIDAggregation.Source()
		require.NoError(t, err)

		expected := make(map[string]any)
		json.Unmarshal([]byte(expectedStr), &expected)
		expected["terms"].(map[string]any)["size"] = 123
		expected["terms"].(map[string]any)["order"] = []any{map[string]string{"startTime": "desc"}}
		assert.EqualValues(t, expected, actual)
	})
}

func TestSpanReader_buildFindTraceIDsQuery(t *testing.T) {
	withSpanReader(t, func(r *spanReaderTest) {
		traceQuery := dbmodel.TraceQueryParameters{
			DurationMin:   time.Second,
			DurationMax:   time.Second * 2,
			StartTimeMin:  time.Time{},
			StartTimeMax:  time.Time{}.Add(time.Second),
			ServiceName:   "s",
			OperationName: "o",
			Tags: map[string]string{
				"hello": "world",
			},
		}

		actualQuery := r.reader.buildFindTraceIDsQuery(traceQuery)
		actual, err := actualQuery.Source()
		require.NoError(t, err)
		expectedQuery := esquery.NewBoolQuery().
			Must(
				r.reader.buildDurationQuery(time.Second, time.Second*2),
				r.reader.buildStartTimeQuery(time.Time{}, time.Time{}.Add(time.Second)),
				r.reader.buildServiceNameQuery("s"),
				r.reader.buildOperationNameQuery("o"),
				r.reader.buildTagQuery("hello", "world"),
			)
		expected, err := expectedQuery.Source()
		require.NoError(t, err)
		assert.Equal(t, expected, actual)
	})
}

func TestSpanReader_buildDurationQuery(t *testing.T) {
	expectedStr := `{ "range":
			{ "duration": {
				        "gte": 1000000,
				        "lte": 2000000 }
			}
		}`
	withSpanReader(t, func(r *spanReaderTest) {
		durationMin := time.Second
		durationMax := time.Second * 2
		durationQuery := r.reader.buildDurationQuery(durationMin, durationMax)
		actual, err := durationQuery.Source()
		require.NoError(t, err)

		expected := make(map[string]any)
		json.Unmarshal([]byte(expectedStr), &expected)
		// We need to do this because we cannot process a json into uint64.
		expected["range"].(map[string]any)["duration"].(map[string]any)["gte"] = model.DurationAsMicroseconds(durationMin)
		expected["range"].(map[string]any)["duration"].(map[string]any)["lte"] = model.DurationAsMicroseconds(durationMax)

		assert.EqualValues(t, expected, actual)
	})
}

func TestSpanReader_buildStartTimeQuery(t *testing.T) {
	expectedStr := `{ "range":
			{ "startTimeMillis": {
				         "gte": 1000000,
				         "lte": 2000000 }
			}
		}`
	withSpanReader(t, func(r *spanReaderTest) {
		startTimeMin := time.Time{}.Add(time.Second)
		startTimeMax := time.Time{}.Add(2 * time.Second)
		durationQuery := r.reader.buildStartTimeQuery(startTimeMin, startTimeMax)
		actual, err := durationQuery.Source()
		require.NoError(t, err)

		expected := make(map[string]any)
		json.Unmarshal([]byte(expectedStr), &expected)
		// We need to do this because we cannot process a json into uint64.
		expected["range"].(map[string]any)["startTimeMillis"].(map[string]any)["gte"] = model.TimeAsEpochMicroseconds(startTimeMin) / 1000
		expected["range"].(map[string]any)["startTimeMillis"].(map[string]any)["lte"] = model.TimeAsEpochMicroseconds(startTimeMax) / 1000

		assert.EqualValues(t, expected, actual)
	})
}

func TestSpanReader_buildServiceNameQuery(t *testing.T) {
	expectedStr := `{ "match": { "process.serviceName": { "query": "bat" }}}`
	withSpanReader(t, func(r *spanReaderTest) {
		serviceNameQuery := r.reader.buildServiceNameQuery("bat")
		actual, err := serviceNameQuery.Source()
		require.NoError(t, err)

		expected := make(map[string]any)
		json.Unmarshal([]byte(expectedStr), &expected)

		assert.EqualValues(t, expected, actual)
	})
}

func TestSpanReader_buildOperationNameQuery(t *testing.T) {
	expectedStr := `{ "match": { "operationName": { "query": "spook" }}}`
	withSpanReader(t, func(r *spanReaderTest) {
		operationNameQuery := r.reader.buildOperationNameQuery("spook")
		actual, err := operationNameQuery.Source()
		require.NoError(t, err)

		expected := make(map[string]any)
		json.Unmarshal([]byte(expectedStr), &expected)

		assert.EqualValues(t, expected, actual)
	})
}

func TestSpanReader_buildTagQuery(t *testing.T) {
	inStr, err := os.ReadFile("fixtures/query_01.json")
	require.NoError(t, err)
	withSpanReader(t, func(r *spanReaderTest) {
		tagQuery := r.reader.buildTagQuery("bat.foo", "spook")
		actual, err := tagQuery.Source()
		require.NoError(t, err)

		expected := make(map[string]any)
		json.Unmarshal(inStr, &expected)

		assert.EqualValues(t, expected, actual)
	})
}

func TestSpanReader_buildTagRegexQuery(t *testing.T) {
	inStr, err := os.ReadFile("fixtures/query_02.json")
	require.NoError(t, err)
	withSpanReader(t, func(r *spanReaderTest) {
		tagQuery := r.reader.buildTagQuery("bat.foo", "spo.*")
		actual, err := tagQuery.Source()
		require.NoError(t, err)

		expected := make(map[string]any)
		json.Unmarshal(inStr, &expected)

		assert.EqualValues(t, expected, actual)
	})
}

func TestSpanReader_buildTagRegexEscapedQuery(t *testing.T) {
	inStr, err := os.ReadFile("fixtures/query_03.json")
	require.NoError(t, err)
	withSpanReader(t, func(r *spanReaderTest) {
		tagQuery := r.reader.buildTagQuery("bat.foo", "spo\\*")
		actual, err := tagQuery.Source()
		require.NoError(t, err)

		expected := make(map[string]any)
		json.Unmarshal(inStr, &expected)

		assert.EqualValues(t, expected, actual)
	})
}

func TestSpanReader_GetEmptyIndex(t *testing.T) {
	withSpanReader(t, func(r *spanReaderTest) {
		mockSearchService(r).Return(&esclient.SearchResponse{}, nil)

		traceQuery := dbmodel.TraceQueryParameters{
			ServiceName: serviceName,
			Tags: map[string]string{
				"hello": "world",
			},
			StartTimeMin: time.Now().Add(-1 * time.Hour),
			StartTimeMax: time.Now(),
			SearchDepth:  2,
		}

		services, err := r.reader.FindTraces(context.Background(), traceQuery)
		require.NotEmpty(t, r.traceBuffer.GetSpans(), "Spans recorded")
		require.NoError(t, err)
		assert.Empty(t, services)
	})
}

func TestSpanReader_ArchiveTraces(t *testing.T) {
	testCases := []struct {
		useAliases bool
		suffix     string
	}{
		{false, ""},
		{true, ""},
		{false, "foobar"},
		{true, "foobar"},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("useAliases=%v suffix=%s", tc.useAliases, tc.suffix), func(t *testing.T) {
			withArchiveSpanReader(t, tc.useAliases, tc.suffix, func(r *spanReaderTest) {
				// An empty trace-ID list short-circuits multiRead before any search,
				// so no searcher call is expected regardless of the rotation config.
				query := []dbmodel.TraceID{}
				trace, err := r.reader.GetTraces(context.Background(), query)
				require.NoError(t, err)
				require.NotEmpty(t, r.traceBuffer.GetSpans(), "Spans recorded")
				require.Empty(t, trace)
			})
		})
	}
}

func TestBuildTraceByIDQuery(t *testing.T) {
	tests := []struct {
		name          string
		traceID       string
		disableLegacy bool
		expected      any
	}{
		{
			name:          "leading zero, legacy disabled",
			traceID:       "0000000000000001",
			disableLegacy: true,
			expected:      map[string]any{"term": map[string]any{"traceID": "0000000000000001"}},
		},
		{
			name:          "long id, legacy disabled",
			traceID:       "00000000000000010000000000000001",
			disableLegacy: true,
			expected:      map[string]any{"term": map[string]any{"traceID": "00000000000000010000000000000001"}},
		},
		{
			name:          "no leading zero",
			traceID:       "ffffffffffffffffffffffffffffffff",
			disableLegacy: true,
			expected:      map[string]any{"term": map[string]any{"traceID": "ffffffffffffffffffffffffffffffff"}},
		},
		{
			name:          "leading zero non-hex, legacy disabled",
			traceID:       "0short-traceid",
			disableLegacy: true,
			expected:      map[string]any{"term": map[string]any{"traceID": "0short-traceid"}},
		},
		{
			// Legacy branch: a 0-prefixed id also matches its leading-zeros-trimmed form.
			name:          "leading zero, legacy enabled",
			traceID:       "0000000000000001",
			disableLegacy: false,
			expected: map[string]any{"bool": map[string]any{"should": []any{
				map[string]any{"term": map[string]any{"traceID": map[string]any{"value": "0000000000000001", "boost": float64(2)}}},
				map[string]any{"term": map[string]any{"traceID": "1"}},
			}}},
		},
		{
			// An empty ID must not panic (traceIDStr[0]); it yields a match-nothing term.
			name:          "empty id, legacy enabled",
			traceID:       "",
			disableLegacy: false,
			expected:      map[string]any{"term": map[string]any{"traceID": ""}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			q := buildTraceByIDQueryWithLegacy(dbmodel.TraceID(test.traceID), test.disableLegacy)
			actual, err := q.Source()
			require.NoError(t, err)
			assert.Equal(t, test.expected, actual)
		})
	}
}

func TestTagsMap(t *testing.T) {
	tests := []struct {
		fieldTags map[string]any
		expected  dbmodel.KeyValue
	}{
		{fieldTags: map[string]any{"bool:bool": true}, expected: dbmodel.KeyValue{Key: "bool.bool", Value: true, Type: dbmodel.BoolType}},
		{fieldTags: map[string]any{"int.int": int64(1)}, expected: dbmodel.KeyValue{Key: "int.int", Value: int64(1), Type: dbmodel.Int64Type}},
		{fieldTags: map[string]any{"int:int": int64(2)}, expected: dbmodel.KeyValue{Key: "int.int", Value: int64(2), Type: dbmodel.Int64Type}},
		{fieldTags: map[string]any{"float": float64(1.1)}, expected: dbmodel.KeyValue{Key: "float", Value: float64(1.1), Type: dbmodel.Float64Type}},
		{fieldTags: map[string]any{"float": float64(123)}, expected: dbmodel.KeyValue{Key: "float", Value: float64(123), Type: dbmodel.Float64Type}},
		{fieldTags: map[string]any{"float": float64(123.0)}, expected: dbmodel.KeyValue{Key: "float", Value: float64(123.0), Type: dbmodel.Float64Type}},
		{fieldTags: map[string]any{"float:float": float64(123)}, expected: dbmodel.KeyValue{Key: "float.float", Value: float64(123), Type: dbmodel.Float64Type}},
		{fieldTags: map[string]any{"json_number:int": json.Number("123")}, expected: dbmodel.KeyValue{Key: "json_number.int", Value: int64(123), Type: dbmodel.Int64Type}},
		{fieldTags: map[string]any{"json_number:float": json.Number("123.0")}, expected: dbmodel.KeyValue{Key: "json_number.float", Value: float64(123.0), Type: dbmodel.Float64Type}},
		{fieldTags: map[string]any{"json_number:err": json.Number("foo")}, expected: dbmodel.KeyValue{Key: "json_number.err", Value: "invalid tag type in foo: strconv.ParseFloat: parsing \"foo\": invalid syntax", Type: dbmodel.StringType}},
		{fieldTags: map[string]any{"str": "foo"}, expected: dbmodel.KeyValue{Key: "str", Value: "foo", Type: dbmodel.StringType}},
		{fieldTags: map[string]any{"str:str": "foo"}, expected: dbmodel.KeyValue{Key: "str.str", Value: "foo", Type: dbmodel.StringType}},
		{fieldTags: map[string]any{"binary": []byte("foo")}, expected: dbmodel.KeyValue{Key: "binary", Value: []byte("foo"), Type: dbmodel.BinaryType}},
		{fieldTags: map[string]any{"binary:binary": []byte("foo")}, expected: dbmodel.KeyValue{Key: "binary.binary", Value: []byte("foo"), Type: dbmodel.BinaryType}},
		{fieldTags: map[string]any{"unsupported": struct{}{}}, expected: dbmodel.KeyValue{Key: "unsupported", Value: fmt.Sprintf("invalid tag type in %+v", struct{}{}), Type: dbmodel.StringType}},
	}
	reader := NewSpanReader(SpanReaderParams{
		TagDotReplacement: ":",
		Logger:            zap.NewNop(),
	})
	for i, test := range tests {
		t.Run(fmt.Sprintf("%d, %s", i, test.fieldTags), func(t *testing.T) {
			tags := []dbmodel.KeyValue{
				{
					Key:   "testing-key",
					Type:  dbmodel.StringType,
					Value: "testing-value",
				},
			}
			spanTags := make(map[string]any)
			maps.Copy(spanTags, test.fieldTags)
			span := &dbmodel.Span{
				Process: dbmodel.Process{
					Tag:  test.fieldTags,
					Tags: tags,
				},
				Tag:  spanTags,
				Tags: tags,
			}
			reader.mergeAllNestedAndElevatedTagsOfSpan(span)
			tags = append(tags, test.expected)
			assert.Empty(t, span.Tag)
			assert.Empty(t, span.Process.Tag)
			assert.Equal(t, tags, span.Tags)
			assert.Equal(t, tags, span.Process.Tags)
		})
	}
}

// newSnapshotReader builds a SpanReader wired to searcher, with aliased (fixed)
// index names so the recorded request paths are deterministic across runs.
func newSnapshotReader(searcher esclient.Searcher) *SpanReader {
	return NewSpanReader(SpanReaderParams{
		Searcher:         searcher,
		MaxSpanAge:       72 * time.Hour,
		MaxTraceDuration: 24 * time.Hour,
		MaxDocCount:      100,
		Logger:           zap.NewNop(),
		Tracer:           noop.NewTracerProvider().Tracer("test"),
		SpanRotation:     indices.NewAliasedRotation("jaeger-span-write-000001", "jaeger-span-read"),
		ServiceRotation:  indices.NewAliasedRotation("jaeger-service-write-000001", "jaeger-service-read"),
	})
}

// TestReaderRequestSnapshots freezes the wire format of the trace-read path:
// find_trace_ids (_search with a terms aggregation) and get_traces (_msearch
// with search_after). Fixed query times keep the range filters deterministic.
func TestReaderRequestSnapshots(t *testing.T) {
	start := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	end := start.Add(time.Hour)
	traceQuery := dbmodel.TraceQueryParameters{
		ServiceName:   "test-service",
		OperationName: "test-operation",
		Tags:          map[string]string{"http.status_code": "200"},
		StartTimeMin:  start,
		StartTimeMax:  end,
		DurationMin:   time.Second,
		DurationMax:   time.Minute,
		SearchDepth:   20,
	}
	traceIDs := []dbmodel.TraceID{"1234567890abcdef"}

	findTraceIDs := map[es.BackendVersion]string{}
	getTraces := map[es.BackendVersion]string{}

	for _, version := range es.AllVersions {
		rec := dataRecorder()
		server := httptest.NewServer(rec)
		t.Cleanup(server.Close)
		esClient, err := esclient.NewClient(context.Background(), &config.Configuration{Servers: []string{server.URL}, Version: uint(version)}, zap.NewNop(), nil)
		require.NoError(t, err)
		searcher := esclient.SearchClient{Client: esClient}
		reader := newSnapshotReader(searcher)
		ctx := context.Background()

		rec.Reset()
		_, err = reader.FindTraceIDs(ctx, traceQuery)
		require.NoError(t, err)
		findTraceIDs[version] = rec.Marshal(t)

		rec.Reset()
		_, err = reader.multiRead(ctx, traceIDs, start, end)
		require.NoError(t, err)
		getTraces[version] = rec.Marshal(t)
	}

	snapshottest.AssertByVersion(t, "testdata/find_trace_ids", findTraceIDs)
	snapshottest.AssertByVersion(t, "testdata/get_traces", getTraces)
}
