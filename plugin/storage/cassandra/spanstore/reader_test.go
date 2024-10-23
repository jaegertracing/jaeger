// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/metricstest"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/cassandra"
	"github.com/jaegertracing/jaeger/pkg/cassandra/mocks"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/plugin/storage/cassandra/spanstore/dbmodel"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

type spanReaderTest struct {
	session     *mocks.Session
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
	session := &mocks.Session{}
	query := &mocks.Query{}
	session.On("Query",
		fmt.Sprintf(tableCheckStmt, schemas[latestVersion].tableName),
		mock.Anything).Return(query)
	query.On("Exec").Return(nil)
	logger, logBuffer := testutils.NewLogger()
	metricsFactory := metricstest.NewFactory(0)
	tracer, exp, closer := tracerProvider(t)
	defer closer()
	reader, err := NewSpanReader(session, metricsFactory, logger, tracer.Tracer("test"))
	require.NoError(t, err)
	r := &spanReaderTest{
		session:     session,
		logger:      logger,
		logBuffer:   logBuffer,
		traceBuffer: exp,
		reader:      reader,
	}
	fn(r)
}

var _ spanstore.Reader = &SpanReader{} // check API conformance

func TestNewSpanReader(t *testing.T) {
	t.Run("test span reader creation", func(t *testing.T) {
		withSpanReader(t, func(r *spanReaderTest) {
			assert.NotNil(t, r.reader)
		})
	})

	t.Run("test span reader creation error", func(t *testing.T) {
		session := &mocks.Session{}
		query := &mocks.Query{}
		session.On("Query",
			fmt.Sprintf(tableCheckStmt, schemas[latestVersion].tableName),
			mock.Anything).Return(query)
		session.On("Query",
			fmt.Sprintf(tableCheckStmt, schemas[previousVersion].tableName),
			mock.Anything).Return(query)
		query.On("Exec").Return(errors.New("table does not exist"))
		logger, _ := testutils.NewLogger()
		metricsFactory := metricstest.NewFactory(0)
		tracer, _, closer := tracerProvider(t)
		defer closer()

		_, err := NewSpanReader(session, metricsFactory, logger, tracer.Tracer("test"))

		require.EqualError(t, err, "neither table operation_names_v2 nor operation_names exist")
	})
}

func TestSpanReaderGetServices(t *testing.T) {
	withSpanReader(t, func(r *spanReaderTest) {
		r.reader.serviceNamesReader = func() ([]string, error) { return []string{"service-a"}, nil }
		s, err := r.reader.GetServices(context.Background())
		require.NoError(t, err)
		assert.Equal(t, []string{"service-a"}, s)
	})
}

func TestSpanReaderGetOperations(t *testing.T) {
	withSpanReader(t, func(r *spanReaderTest) {
		expectedOperations := []spanstore.Operation{
			{
				Name:     "operation-a",
				SpanKind: "server",
			},
		}
		r.reader.operationNamesReader = func(_ spanstore.OperationQueryParameters) ([]spanstore.Operation, error) {
			return expectedOperations, nil
		}
		s, err := r.reader.GetOperations(context.Background(),
			spanstore.OperationQueryParameters{ServiceName: "service-x", SpanKind: "server"})
		require.NoError(t, err)
		assert.Equal(t, expectedOperations, s)
	})
}

func TestSpanReaderGetTrace(t *testing.T) {
	badScan := func() any {
		return matchOnceWithSideEffect(func(args []any) {
			for _, arg := range args {
				if v, ok := arg.(*[]dbmodel.KeyValue); ok {
					*v = []dbmodel.KeyValue{
						{
							ValueType: "bad",
						},
					}
				}
			}
		})
	}

	testCases := []struct {
		scanner     any
		closeErr    error
		expectedErr string
	}{
		{scanner: matchOnce()},
		{scanner: badScan(), expectedErr: "invalid ValueType in"},
		{
			scanner:     matchOnce(),
			closeErr:    errors.New("error on close()"),
			expectedErr: "error reading traces from storage: error on close()",
		},
	}
	for _, tc := range testCases {
		testCase := tc // capture loop var
		t.Run("expected err="+testCase.expectedErr, func(t *testing.T) {
			withSpanReader(t, func(r *spanReaderTest) {
				iter := &mocks.Iterator{}
				iter.On("Scan", testCase.scanner).Return(true)
				iter.On("Scan", matchEverything()).Return(false)
				iter.On("Close").Return(testCase.closeErr)

				query := &mocks.Query{}
				query.On("Consistency", cassandra.One).Return(query)
				query.On("Iter").Return(iter)

				r.session.On("Query", mock.AnythingOfType("string"), matchEverything()).Return(query)

				trace, err := r.reader.GetTrace(context.Background(), model.TraceID{})
				if testCase.expectedErr == "" {
					require.NotEmpty(t, r.traceBuffer.GetSpans(), "Spans recorded")
					require.NoError(t, err)
					assert.NotNil(t, trace)
				} else {
					require.Error(t, err)
					assert.Contains(t, err.Error(), testCase.expectedErr)
					assert.Nil(t, trace)
				}
			})
		})
	}
}

func TestSpanReaderGetTrace_TraceNotFound(t *testing.T) {
	withSpanReader(t, func(r *spanReaderTest) {
		iter := &mocks.Iterator{}
		iter.On("Scan", matchEverything()).Return(false)
		iter.On("Close").Return(nil)

		query := &mocks.Query{}
		query.On("Consistency", cassandra.One).Return(query)
		query.On("Iter").Return(iter)

		r.session.On("Query", mock.AnythingOfType("string"), matchEverything()).Return(query)

		trace, err := r.reader.GetTrace(context.Background(), model.TraceID{})
		require.NotEmpty(t, r.traceBuffer.GetSpans(), "Spans recorded")
		assert.Nil(t, trace)
		require.EqualError(t, err, "trace not found")
	})
}

func TestSpanReaderFindTracesBadRequest(t *testing.T) {
	withSpanReader(t, func(r *spanReaderTest) {
		_, err := r.reader.FindTraces(context.Background(), nil)
		require.Empty(t, r.traceBuffer.GetSpans(), "Spans Not recorded")
		require.Error(t, err)
	})
}

func TestSpanReaderFindTraces(t *testing.T) {
	testCases := []struct {
		caption                           string
		numTraces                         int
		queryTags                         bool
		queryOperation                    bool
		queryDuration                     bool
		mainQueryError                    error
		tagsQueryError                    error
		serviceNameAndOperationQueryError error
		durationQueryError                error
		loadQueryError                    error
		expectedCount                     int
		expectedError                     string
		expectedLogs                      []string
	}{
		{
			caption:       "main query",
			expectedCount: 2,
		},
		{
			caption:       "tag query",
			expectedCount: 2,
			queryTags:     true,
		},
		{
			caption:       "with limit",
			numTraces:     1,
			expectedCount: 1,
		},
		{
			caption:        "main query error",
			mainQueryError: errors.New("main query error"),
			expectedError:  "main query error",
			expectedLogs: []string{
				"Failed to exec query",
				"main query error",
			},
		},
		{
			caption:        "tags query error",
			queryTags:      true,
			tagsQueryError: errors.New("tags query error"),
			expectedError:  "tags query error",
			expectedLogs: []string{
				"Failed to exec query",
				"tags query error",
			},
		},
		{
			caption:        "operation name query",
			queryOperation: true,
			numTraces:      0,
			expectedCount:  2,
		},
		{
			caption:        "operation name and tag query",
			queryTags:      true,
			queryOperation: true,
			expectedCount:  2,
		},
		{
			caption:                           "operation name and tag error on operation query",
			queryTags:                         true,
			queryOperation:                    true,
			serviceNameAndOperationQueryError: errors.New("operation query error"),
			expectedError:                     "operation query error",
			expectedLogs: []string{
				"Failed to exec query",
				"operation query error",
			},
		},
		{
			caption:        "operation name and tag error on tag query",
			queryTags:      true,
			queryOperation: true,
			tagsQueryError: errors.New("tags query error"),
			expectedError:  "tags query error",
			expectedLogs: []string{
				"Failed to exec query",
				"tags query error",
			},
		},
		{
			caption:       "duration query",
			queryDuration: true,
			numTraces:     1,
			expectedCount: 1,
		},
		{
			caption:            "duration query error",
			queryDuration:      true,
			durationQueryError: errors.New("duration query error"),
			expectedError:      "duration query error",
			expectedLogs: []string{
				"Failed to exec query",
				"duration query error",
			},
		},
		{
			caption:        "load trace error",
			loadQueryError: errors.New("load query error"),
			expectedCount:  0,
			expectedLogs: []string{
				"Failure to read trace",
				"error reading traces from storage: load query error",
				`"trace_id":"0000000000000001"`,
				`"trace_id":"0000000000000002"`,
			},
		},
	}
	for _, tc := range testCases {
		testCase := tc // capture loop var
		t.Run(testCase.caption, func(t *testing.T) {
			withSpanReader(t, func(r *spanReaderTest) {
				// scanMatcher can match Iter.Scan() parameters and set trace ID fields
				scanMatcher := func(_ /* name */ string) any {
					traceIDs := []dbmodel.TraceID{
						dbmodel.TraceIDFromDomain(model.NewTraceID(0, 1)),
						dbmodel.TraceIDFromDomain(model.NewTraceID(0, 2)),
					}
					scanFunc := func(args []any) bool {
						if len(traceIDs) == 0 {
							return false
						}
						for _, arg := range args {
							if ptr, ok := arg.(*dbmodel.TraceID); ok {
								*ptr = traceIDs[0]
								break
							}
						}
						traceIDs = traceIDs[1:]
						return true
					}
					return mock.MatchedBy(scanFunc)
				}

				mockQuery := func(queryErr error) *mocks.Query {
					iter := &mocks.Iterator{}
					iter.On("Scan", scanMatcher("queryIter")).Return(true)
					iter.On("Scan", matchEverything()).Return(false)
					iter.On("Close").Return(queryErr)

					query := &mocks.Query{}
					query.On("Bind", matchEverything()).Return(query)
					query.On("Consistency", cassandra.One).Return(query)
					query.On("PageSize", 0).Return(query)
					query.On("Iter").Return(iter)
					query.On("String").Return("queryString")

					return query
				}

				mainQuery := mockQuery(testCase.mainQueryError)
				tagsQuery := mockQuery(testCase.tagsQueryError)
				operationQuery := mockQuery(testCase.serviceNameAndOperationQueryError)
				durationQuery := mockQuery(testCase.durationQueryError)

				makeLoadQuery := func() *mocks.Query {
					loadQueryIter := &mocks.Iterator{}
					loadQueryIter.On("Scan", scanMatcher("loadIter")).Return(true)
					loadQueryIter.On("Scan", matchEverything()).Return(false)
					loadQueryIter.On("Close").Return(testCase.loadQueryError)

					loadQuery := &mocks.Query{}
					loadQuery.On("Consistency", cassandra.One).Return(loadQuery)
					loadQuery.On("Iter").Return(loadQueryIter)
					loadQuery.On("PageSize", matchEverything()).Return(loadQuery)
					return loadQuery
				}

				r.session.On("Query",
					stringMatcher(queryByServiceName),
					matchEverything()).Return(mainQuery)
				r.session.On("Query",
					stringMatcher(queryByTag),
					matchEverything()).Return(tagsQuery)
				r.session.On("Query",
					stringMatcher(queryByServiceAndOperationName),
					matchEverything()).Return(operationQuery)
				r.session.On("Query",
					stringMatcher(queryByDuration),
					matchEverything()).Return(durationQuery)
				r.session.On("Query",
					stringMatcher("SELECT trace_id"),
					matchOnce()).Return(makeLoadQuery())
				r.session.On("Query",
					stringMatcher("SELECT trace_id"),
					matchEverything()).Return(makeLoadQuery())

				queryParams := &spanstore.TraceQueryParameters{
					ServiceName:  "service-a",
					NumTraces:    100,
					StartTimeMax: time.Now(),
					StartTimeMin: time.Now().Add(-1 * time.Minute * 30),
				}

				queryParams.NumTraces = testCase.numTraces
				if testCase.queryTags {
					queryParams.Tags = make(map[string]string)
					queryParams.Tags["x"] = "y"
				}
				if testCase.queryOperation {
					queryParams.OperationName = "operation-b"
				}
				if testCase.queryDuration {
					queryParams.DurationMin = time.Minute
					queryParams.DurationMax = time.Minute * 3
				}
				res, err := r.reader.FindTraces(context.Background(), queryParams)
				if testCase.expectedError == "" {
					require.NotEmpty(t, r.traceBuffer.GetSpans(), "Spans recorded")
					require.NoError(t, err)
					assert.Len(t, res, testCase.expectedCount, "expecting certain number of traces")
				} else {
					require.EqualError(t, err, testCase.expectedError)
				}
				for _, expectedLog := range testCase.expectedLogs {
					assert.Contains(t, r.logBuffer.String(), expectedLog)
				}
				if len(testCase.expectedLogs) == 0 {
					assert.Equal(t, "", r.logBuffer.String())
				}
			})
		})
	}
}

func TestTraceQueryParameterValidation(t *testing.T) {
	tsp := &spanstore.TraceQueryParameters{
		ServiceName: "",
		Tags: map[string]string{
			"michael": "jackson",
		},
	}
	err := validateQuery(tsp)
	require.EqualError(t, err, ErrServiceNameNotSet.Error())

	tsp.ServiceName = "serviceName"
	tsp.StartTimeMin = time.Now()
	tsp.StartTimeMax = time.Now().Add(-1 * time.Hour)
	err = validateQuery(tsp)
	require.EqualError(t, err, ErrStartTimeMinGreaterThanMax.Error())

	tsp.StartTimeMin = time.Now().Add(-12 * time.Hour)
	tsp.DurationMin = time.Hour
	tsp.DurationMax = time.Minute
	err = validateQuery(tsp)
	require.EqualError(t, err, ErrDurationMinGreaterThanMax.Error())

	tsp.DurationMin = time.Minute
	tsp.DurationMax = time.Hour
	err = validateQuery(tsp)
	require.EqualError(t, err, ErrDurationAndTagQueryNotSupported.Error())

	tsp.StartTimeMin = time.Time{} // time.Unix(0,0) doesn't work because timezones
	tsp.StartTimeMax = time.Time{}
	err = validateQuery(tsp)
	require.EqualError(t, err, ErrStartAndEndTimeNotSet.Error())
}
