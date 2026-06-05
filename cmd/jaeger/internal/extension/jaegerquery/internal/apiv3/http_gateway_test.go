// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package apiv3

import (
	"bytes"
	"encoding/json"
	"errors"
	"iter"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"

	"github.com/gogo/protobuf/jsonpb"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/proto/api_v3"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
	dependencystoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	tracestoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore/mocks"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

func setupHTTPGatewayNoServer(
	_ *testing.T,
	basePath string,
) *testGateway {
	gw := &testGateway{
		reader: &tracestoremocks.Reader{},
	}

	q := querysvc.NewQueryService(
		gw.reader,
		&dependencystoremocks.Reader{},
		querysvc.QueryServiceOptions{},
	)

	hgw := &HTTPGateway{
		QueryService: q,
		Logger:       zap.NewNop(),
		Tracer:       nooptrace.NewTracerProvider(),
	}

	gw.router = http.NewServeMux()
	hgw.RegisterRoutes(gw.router)
	return gw
}

func setupHTTPGateway(
	t *testing.T,
	basePath string,
) *testGateway {
	gw := setupHTTPGatewayNoServer(t, basePath)

	httpServer := httptest.NewServer(gw.router)
	t.Cleanup(func() { httpServer.Close() })

	gw.url = httpServer.URL
	if basePath != "/" {
		gw.url += basePath
	}
	return gw
}

func TestHTTPGateway(t *testing.T) {
	runGatewayTests(t, "/", func(_ *http.Request) {})
}

func TestHTTPGatewayTryHandleError(t *testing.T) {
	gw := new(HTTPGateway)
	assert.False(t, gw.tryHandleError(nil, nil, 0), "returns false if no error")

	w := httptest.NewRecorder()
	assert.True(t, gw.tryHandleError(w, spanstore.ErrTraceNotFound, 0), "returns true if error")
	assert.Equal(t, http.StatusNotFound, w.Code, "sets status code to 404")

	logger, log := testutils.NewLogger()
	gw.Logger = logger
	w = httptest.NewRecorder()
	const e = "some err"
	assert.True(t, gw.tryHandleError(w, errors.New(e), http.StatusInternalServerError))
	assert.Contains(t, log.String(), e, "logs error if status code is 500")
	assert.Contains(t, string(w.Body.String()), e, "writes error message to body")
}

func TestHTTPGatewayGetTrace(t *testing.T) {
	testCases := []struct {
		name          string
		params        map[string]string
		expectedQuery tracestore.GetTraceParams
	}{
		{
			name:   "no params",
			params: map[string]string{},
			expectedQuery: tracestore.GetTraceParams{
				TraceID: traceID,
			},
		},
		{
			name: "camelCase time window",
			params: map[string]string{
				"startTime": "2000-01-02T12:30:08.999999998Z",
				"endTime":   "2000-04-05T21:55:16.999999992+08:00",
			},
			expectedQuery: tracestore.GetTraceParams{
				TraceID: traceID,
				Start:   time.Date(2000, time.January, 2, 12, 30, 8, 999999998, time.UTC),
				End:     time.Date(2000, time.April, 5, 13, 55, 16, 999999992, time.UTC),
			},
		},
		{
			name: "deprecated snake_case time window",
			params: map[string]string{
				"start_time": "2000-01-02T12:30:08.999999998Z",
				"end_time":   "2000-04-05T21:55:16.999999992+08:00",
			},
			expectedQuery: tracestore.GetTraceParams{
				TraceID: traceID,
				Start:   time.Date(2000, time.January, 2, 12, 30, 8, 999999998, time.UTC),
				End:     time.Date(2000, time.April, 5, 13, 55, 16, 999999992, time.UTC),
			},
		},
	}

	testUri := "/api/v3/traces/1"

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gw := setupHTTPGatewayNoServer(t, "")
			gw.reader.
				On("GetTraces", matchContext, []tracestore.GetTraceParams{tc.expectedQuery}).
				Return(iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
					yield([]ptrace.Traces{makeTestTrace()}, nil)
				})).Once()

			q := url.Values{}
			for k, v := range tc.params {
				q.Set(k, v)
			}
			testUrl := testUri
			if len(tc.params) > 0 {
				testUrl += "?" + q.Encode()
			}

			r, err := http.NewRequest(http.MethodGet, testUrl, http.NoBody)
			require.NoError(t, err)
			w := httptest.NewRecorder()
			gw.router.ServeHTTP(w, r)
			gw.reader.AssertCalled(t, "GetTraces", matchContext, []tracestore.GetTraceParams{tc.expectedQuery})
		})
	}
}

func TestHTTPGatewayGetTraceEmptyResponse(t *testing.T) {
	gw := setupHTTPGatewayNoServer(t, "")
	gw.reader.On("GetTraces", matchContext, mock.AnythingOfType("[]tracestore.GetTraceParams")).
		Return(iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
			yield([]ptrace.Traces{}, nil)
		})).Once()

	r, err := http.NewRequest(http.MethodGet, "/api/v3/traces/1", http.NoBody)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	gw.router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "No traces found")
}

func TestHTTPGatewayFindTracesEmptyResponse(t *testing.T) {
	q, qp := mockFindQueries()
	r, err := http.NewRequest(http.MethodGet, "/api/v3/traces?"+q.Encode(), http.NoBody)
	require.NoError(t, err)
	w := httptest.NewRecorder()

	gw := setupHTTPGatewayNoServer(t, "")
	gw.reader.
		On("FindTraces", matchContext, qp).
		Return(iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
			yield([]ptrace.Traces{}, nil)
		})).Once()

	gw.router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "No traces found")
}

// TestHTTPGatewayFindTracesDeprecatedParams verifies that deprecated snake_case query params
// are accepted as fallbacks for the canonical camelCase params.
func TestHTTPGatewayFindTracesDeprecatedParams(t *testing.T) {
	_, qp := mockFindQueries()
	// Build query using deprecated snake_case param names.
	time1 := qp.StartTimeMin
	time2 := qp.StartTimeMax
	q := url.Values{}
	q.Set("query.service_name", "foo")
	q.Set("query.operation_name", "bar")
	q.Set("query.start_time_min", time1.Format(time.RFC3339Nano))
	q.Set("query.start_time_max", time2.Format(time.RFC3339Nano))
	q.Set("query.duration_min", "1s")
	q.Set("query.duration_max", "2s")
	q.Set("query.search_depth", "10")

	r, err := http.NewRequest(http.MethodGet, "/api/v3/traces?"+q.Encode(), http.NoBody)
	require.NoError(t, err)
	w := httptest.NewRecorder()

	gw := setupHTTPGatewayNoServer(t, "")
	gw.reader.
		On("FindTraces", matchContext, qp).
		Return(iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
			yield([]ptrace.Traces{}, nil)
		})).Once()

	gw.router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "No traces found")
}

// TestHTTPGatewayFindTracesDeprecatedNumTraces verifies that the deprecated
// query.num_traces alias is accepted as a fallback for query.searchDepth.
func TestHTTPGatewayFindTracesDeprecatedNumTraces(t *testing.T) {
	q, qp := mockFindQueries()
	// Replace canonical searchDepth with the deprecated num_traces alias.
	q.Del("query.searchDepth")
	q.Set("query.num_traces", "10")
	r, err := http.NewRequest(http.MethodGet, "/api/v3/traces?"+q.Encode(), http.NoBody)
	require.NoError(t, err)
	w := httptest.NewRecorder()

	gw := setupHTTPGatewayNoServer(t, "")
	gw.reader.
		On("FindTraces", matchContext, qp).
		Return(iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
			yield([]ptrace.Traces{}, nil)
		})).Once()

	gw.router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "No traces found")
}

func TestHTTPGatewayGetTraceMalformedInputErrors(t *testing.T) {
	testCases := []struct {
		name          string
		requestUrl    string
		expectedError string
	}{
		{
			name:          "bad trace ID",
			requestUrl:    "/api/v3/traces/xyz",
			expectedError: "malformed parameter trace_id",
		},
		{
			name:          "invalid startTime (canonical)",
			requestUrl:    "/api/v3/traces/1?startTime=abc",
			expectedError: "malformed parameter startTime",
		},
		{
			name:          "invalid start_time (deprecated)",
			requestUrl:    "/api/v3/traces/1?start_time=abc",
			expectedError: "malformed parameter start_time",
		},
		{
			name:          "invalid endTime (canonical)",
			requestUrl:    "/api/v3/traces/1?endTime=xyz",
			expectedError: "malformed parameter endTime",
		},
		{
			name:          "invalid end_time (deprecated)",
			requestUrl:    "/api/v3/traces/1?end_time=xyz",
			expectedError: "malformed parameter end_time",
		},
		{
			name:          "invalid rawTraces (canonical)",
			requestUrl:    "/api/v3/traces/1?rawTraces=foobar",
			expectedError: "malformed parameter rawTraces",
		},
		{
			name:          "invalid raw_traces (deprecated)",
			requestUrl:    "/api/v3/traces/1?raw_traces=foobar",
			expectedError: "malformed parameter raw_traces",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gw := setupHTTPGatewayNoServer(t, "")
			gw.reader.On("GetTraces", matchContext, mock.AnythingOfType("tracestore.GetTraceParams")).
				Return(iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
					yield([]ptrace.Traces{}, nil)
				})).Once()

			r, err := http.NewRequest(http.MethodGet, tc.requestUrl, http.NoBody)
			require.NoError(t, err)
			w := httptest.NewRecorder()
			gw.router.ServeHTTP(w, r)
			assert.Contains(t, w.Body.String(), tc.expectedError)
		})
	}
}

func TestHTTPGatewayGetTraceInternalErrors(t *testing.T) {
	gw := setupHTTPGatewayNoServer(t, "")
	gw.reader.On("GetTraces", matchContext, mock.AnythingOfType("[]tracestore.GetTraceParams")).
		Return(iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
			yield([]ptrace.Traces{}, assert.AnError)
		})).Once()

	r, err := http.NewRequest(http.MethodGet, "/api/v3/traces/1", http.NoBody)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	gw.router.ServeHTTP(w, r)
	assert.Contains(t, w.Body.String(), assert.AnError.Error())
}

func mockFindQueries() (url.Values, tracestore.TraceQueryParams) {
	// Truncate monotonic clock and force UTC to avoid comparison failures in mocks.
	tMin := time.Now().Add(-time.Hour).UTC().Truncate(time.Nanosecond)
	tMax := time.Now().UTC().Truncate(time.Nanosecond)
	q := url.Values{}
	q.Set("query.serviceName", "foo")
	q.Set("query.operationName", "bar")
	q.Set("query.startTimeMin", tMin.Format(time.RFC3339Nano))
	q.Set("query.startTimeMax", tMax.Format(time.RFC3339Nano))
	q.Set("query.durationMin", "1s")
	q.Set("query.durationMax", "2s")
	q.Set("query.searchDepth", "10")

	return q, tracestore.TraceQueryParams{
		ServiceName:   "foo",
		OperationName: "bar",
		Attributes:    pcommon.NewMap(),
		StartTimeMin:  tMin,
		StartTimeMax:  tMax,
		DurationMin:   1 * time.Second,
		DurationMax:   2 * time.Second,
		SearchDepth:   10,
	}
}

func TestHTTPGatewayFindTracesErrors(t *testing.T) {
	t.Run("parse error returns 400", func(t *testing.T) {
		// Detailed parse error cases are covered by TestParseFindTracesQuery.
		// Here we only verify that any parse error is propagated as HTTP 400.
		r, err := http.NewRequest(http.MethodGet, "/api/v3/traces", http.NoBody)
		require.NoError(t, err)
		w := httptest.NewRecorder()

		gw := setupHTTPGatewayNoServer(t, "")
		gw.router.ServeHTTP(w, r)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "query.startTimeMin and query.startTimeMax are required")
	})
	t.Run("span reader error", func(t *testing.T) {
		q, qp := mockFindQueries()
		r, err := http.NewRequest(http.MethodGet, "/api/v3/traces?"+q.Encode(), http.NoBody)
		require.NoError(t, err)
		w := httptest.NewRecorder()

		gw := setupHTTPGatewayNoServer(t, "")
		gw.reader.
			On("FindTraces", matchContext, qp).
			Return(iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
				yield(nil, assert.AnError)
			})).Once()

		gw.router.ServeHTTP(w, r)
		assert.Contains(t, w.Body.String(), assert.AnError.Error())
	})
}

func TestHTTPGatewayFindTracesAttributes(t *testing.T) {
	tMin := time.Now().Add(-time.Hour).UTC().Truncate(time.Nanosecond)
	tMax := time.Now().UTC().Truncate(time.Nanosecond)

	q := url.Values{}
	q.Set(paramServiceName, "svc")
	q.Set(paramTimeMin, tMin.Format(time.RFC3339Nano))
	q.Set(paramTimeMax, tMax.Format(time.RFC3339Nano))
	q.Set(paramAttributes, `{"http.status_code":"200","error":"true"}`)

	gw := setupHTTPGatewayNoServer(t, "")
	gw.reader.
		On("FindTraces", matchContext, mock.MatchedBy(func(qp tracestore.TraceQueryParams) bool {
			v1, ok1 := qp.Attributes.Get("http.status_code")
			v2, ok2 := qp.Attributes.Get("error")
			return qp.ServiceName == "svc" &&
				qp.StartTimeMin.Equal(tMin) &&
				qp.StartTimeMax.Equal(tMax) &&
				qp.SearchDepth == defaultSearchDepth &&
				qp.Attributes.Len() == 2 &&
				ok1 && v1.AsString() == "200" &&
				ok2 && v2.AsString() == "true"
		})).
		Return(iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
			yield([]ptrace.Traces{makeTestTrace()}, nil)
		})).Once()

	r, err := http.NewRequest(http.MethodGet, "/api/v3/traces?"+q.Encode(), http.NoBody)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	gw.router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusOK, w.Code)
	gw.reader.AssertExpectations(t)
}

func TestHTTPGatewayGetServicesErrors(t *testing.T) {
	gw := setupHTTPGatewayNoServer(t, "")

	gw.reader.
		On("GetServices", matchContext).
		Return(nil, assert.AnError).Once()

	r, err := http.NewRequest(http.MethodGet, "/api/v3/services", http.NoBody)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	gw.router.ServeHTTP(w, r)
	assert.Contains(t, w.Body.String(), assert.AnError.Error())
}

func TestHTTPGatewayGetOperationsErrors(t *testing.T) {
	gw := setupHTTPGatewayNoServer(t, "")

	qp := tracestore.OperationQueryParams{ServiceName: "foo", SpanKind: "server"}
	gw.reader.
		On("GetOperations", matchContext, qp).
		Return(nil, assert.AnError).Twice()

	// canonical camelCase
	r, err := http.NewRequest(http.MethodGet, "/api/v3/operations?service=foo&spanKind=server", http.NoBody)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	gw.router.ServeHTTP(w, r)
	assert.Contains(t, w.Body.String(), assert.AnError.Error())

	// deprecated snake_case
	r, err = http.NewRequest(http.MethodGet, "/api/v3/operations?service=foo&span_kind=server", http.NoBody)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	gw.router.ServeHTTP(w, r)
	assert.Contains(t, w.Body.String(), assert.AnError.Error())
}

func TestHTTPGatewayGetServicesEmptyResponse(t *testing.T) {
	gw := setupHTTPGatewayNoServer(t, "")
	gw.reader.
		On("GetServices", matchContext).
		Return(nil, nil).Once()

	r, err := http.NewRequest(http.MethodGet, "/api/v3/services", http.NoBody)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	gw.router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"services":[]}`, w.Body.String())
	gw.reader.AssertExpectations(t)
}

// TestJSONPBFixed64AsDecimalString confirms that gogoproto/jsonpb encodes fixed64
// fields as decimal strings (consistent with proto3 JSON spec and OTLP convention).
// This validates the assumption behind using marshalResponse for FindTraceSummaries.
func TestJSONPBFixed64AsDecimalString(t *testing.T) {
	summary := &api_v3.TraceSummary{
		MinStartTimeUnixNano: 1_000_000_000_000, // 1000s in ns
		MaxEndTimeUnixNano:   1_001_000_000_000, // 1001s in ns
	}
	var buf bytes.Buffer
	require.NoError(t, new(jsonpb.Marshaler).Marshal(&buf, summary))
	var m map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &m))
	// fixed64 must be a decimal string, not a JSON number, to avoid float64 precision
	// loss for nanosecond values above 2^53 in JavaScript.
	assert.Equal(t, "1000000000000", m["minStartTimeUnixNano"])
	assert.Equal(t, "1001000000000", m["maxEndTimeUnixNano"])
}

func TestHTTPGatewayFindTraceSummaries(t *testing.T) {
	q, qp := mockFindQueries()
	gw := setupHTTPGatewayNoServer(t, "")

	trace := makeTestTrace()
	// Ensure the trace has a root span (no parent) so summarizeTrace populates root fields.
	rs := trace.ResourceSpans().At(0)
	rs.Resource().Attributes().PutStr("service.name", "frontend")
	span := rs.ScopeSpans().At(0).Spans().At(0)
	span.SetName("HTTP GET /")
	span.SetParentSpanID(pcommon.SpanID{}) // explicit root

	gw.reader.
		On("FindTraces", matchContext, qp).
		Return(iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
			yield([]ptrace.Traces{trace}, nil)
		})).Once()

	r, err := http.NewRequest(http.MethodGet, "/api/v3/trace-summaries?"+q.Encode(), http.NoBody)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	gw.router.ServeHTTP(w, r)

	require.Equal(t, http.StatusOK, w.Code)
	var resp api_v3.FindTraceSummariesResponse
	require.NoError(t, jsonpb.Unmarshal(w.Body, &resp))
	require.Len(t, resp.Summaries, 1)
	assert.Equal(t, "frontend", resp.Summaries[0].RootServiceName)
	assert.Equal(t, "HTTP GET /", resp.Summaries[0].RootOperationName)
	assert.Equal(t, int32(1), resp.Summaries[0].SpanCount)
}

func TestHTTPGatewayFindTraceSummariesError(t *testing.T) {
	q, qp := mockFindQueries()
	gw := setupHTTPGatewayNoServer(t, "")

	gw.reader.
		On("FindTraces", matchContext, qp).
		Return(iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
			yield(nil, assert.AnError)
		})).Once()

	r, err := http.NewRequest(http.MethodGet, "/api/v3/trace-summaries?"+q.Encode(), http.NoBody)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	gw.router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), assert.AnError.Error())
}
