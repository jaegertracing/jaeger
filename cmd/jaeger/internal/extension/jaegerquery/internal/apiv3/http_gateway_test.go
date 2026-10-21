// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package apiv3

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/components/extension/jaegerquery/queryinterceptor"
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
	// The mock reader models a backend without native trace summaries: FindTraceSummaries
	// yields ErrUnsupported so the query service falls back to FindTraces + aggregation.
	gw.reader.On("FindTraceSummaries", mock.Anything, mock.Anything).
		Return(iter.Seq2[tracestore.PageChunk[[]tracestore.TraceSummary], error](func(yield func(tracestore.PageChunk[[]tracestore.TraceSummary], error) bool) {
			yield(tracestore.PageChunk[[]tracestore.TraceSummary]{}, fmt.Errorf("unsupported: %w", errors.ErrUnsupported))
		})).Maybe()

	// The baseline: a backend that requires a service name. Only service-less searches ask.
	gw.reader.On("SearchCapabilities", mock.Anything).
		Return(tracestore.SearchCapabilities{}, nil).Maybe()
	q := querysvc.NewQueryService(
		gw.reader,
		&dependencystoremocks.Reader{},
		querysvc.QueryServiceOptions{},
	)

	hgw := &HTTPGateway{
		QueryService: q,
		Logger:       zap.NewNop(),
		Tracer:       nooptrace.NewTracerProvider(),
		BasePath:     basePath,
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

func TestHTTPGatewayBasePath(t *testing.T) {
	gw := setupHTTPGatewayNoServer(t, "/jaeger")
	gw.reader.On("GetServices", matchContext).Return([]string{"foo"}, nil).Once()

	r := httptest.NewRequest(http.MethodGet, "/jaeger/api/v3/services", http.NoBody)
	w := httptest.NewRecorder()
	gw.router.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	gw.reader.AssertExpectations(t)
}

func TestHTTPGatewayTryHandleError(t *testing.T) {
	gw := new(HTTPGateway)
	assert.False(t, gw.tryHandleError(nil, nil, 0), "returns false if no error")

	w := httptest.NewRecorder()
	assert.True(t, gw.tryHandleError(w, spanstore.ErrTraceNotFound, 0), "returns true if error")
	assert.Equal(t, http.StatusNotFound, w.Code, "sets status code to 404")

	// A well-formed query this deployment's storage cannot serve is a missing capability, not
	// a server fault and not a malformed request, so it is 501 rather than 500 or 400
	// (RFC 0013 §3.3).
	w = httptest.NewRecorder()
	assert.True(t, gw.tryHandleError(w, querysvc.ErrServiceNameRequired, http.StatusInternalServerError))
	assert.Equal(t, http.StatusNotImplemented, w.Code, "sets status code to 501")
	assert.Contains(t, w.Body.String(), "requires a service name", "explains the limitation")

	// A malformed query is the caller's mistake wherever it is sent, so it is 400.
	w = httptest.NewRecorder()
	assert.True(t, gw.tryHandleError(w, fmt.Errorf("%w: search depth", tracestore.ErrInvalidQuery), http.StatusInternalServerError))
	assert.Equal(t, http.StatusBadRequest, w.Code, "sets status code to 400")

	w = httptest.NewRecorder()
	assert.True(t, gw.tryHandleError(w, fmt.Errorf("denied: %w", queryinterceptor.ErrAccessDenied), http.StatusInternalServerError))
	assert.Equal(t, http.StatusForbidden, w.Code, "ErrAccessDenied maps to 403")

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
		{
			name:   "raw traces",
			params: map[string]string{"rawTraces": "true"},
			expectedQuery: tracestore.GetTraceParams{
				TraceID: traceID,
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
			assert.Equal(t, http.StatusOK, w.Code)
			gw.reader.AssertCalled(t, "GetTraces", matchContext, []tracestore.GetTraceParams{tc.expectedQuery})
		})
	}
}

func TestHTTPGatewayGetTraceBase64(t *testing.T) {
	tests := []struct {
		name    string
		urlPath string
		traceID pcommon.TraceID
	}{
		{
			name:    "standard base64 with padding",
			urlPath: "/api/v3/traces/AAAAAAAAAAAAAAAAAAAAAQ==",
			traceID: traceID, // [16]byte{0,...,0,1}
		},
		{
			// base64 contains "/" which must be percent-encoded in URL path
			name:    "base64 with slash (url-encoded)",
			urlPath: "/api/v3/traces/AAAAAAAAAP%2F%2F%2F%2F%2F%2F%2F%2F%2F%2F%2Fw==",
			traceID: pcommon.TraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}),
		},
		{
			// base64 contains "+" which must be percent-encoded in URL path
			name:    "base64 with plus (url-encoded)",
			urlPath: "/api/v3/traces/EjRWeJq83vD%2B3LqYdlQyEA==",
			traceID: pcommon.TraceID([16]byte{0x12, 0x34, 0x56, 0x78, 0x9A, 0xBC, 0xDE, 0xF0, 0xFE, 0xDC, 0xBA, 0x98, 0x76, 0x54, 0x32, 0x10}),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gw := setupHTTPGatewayNoServer(t, "")
			gw.reader.
				On("GetTraces", matchContext, []tracestore.GetTraceParams{{TraceID: tc.traceID}}).
				Return(iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
					yield([]ptrace.Traces{makeTestTrace()}, nil)
				})).Once()

			r, err := http.NewRequest(http.MethodGet, tc.urlPath, http.NoBody)
			require.NoError(t, err)
			w := httptest.NewRecorder()
			gw.router.ServeHTTP(w, r)
			assert.Equal(t, http.StatusOK, w.Code)
			gw.reader.AssertCalled(t, "GetTraces", matchContext, []tracestore.GetTraceParams{{TraceID: tc.traceID}})
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
	t.Run("missing time range returns 400", func(t *testing.T) {
		// The refusal comes from the query service, not the parser; the gateway has to report it
		// as a bad request rather than a server fault.
		r, err := http.NewRequest(http.MethodGet, "/api/v3/traces", http.NoBody)
		require.NoError(t, err)
		w := httptest.NewRecorder()

		gw := setupHTTPGatewayNoServer(t, "")
		gw.router.ServeHTTP(w, r)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "min and max start time are required")
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
				qp.SearchDepth == querysvc.DefaultSearchDepth &&
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

// TestHTTPGatewayGetCapabilities pins the JSON shape of a full declaration, which is what a
// client or the UI reads.
func TestHTTPGatewayGetCapabilities(t *testing.T) {
	gw := setupHTTPGatewayNoServer(t, "")
	gw.reader.ExpectedCalls = nil
	gw.reader.On("SearchCapabilities", mock.Anything).Return(tracestore.SearchCapabilities{
		WithoutServiceName:  true,
		SameSpanConjunction: true,
		Filter: &tracestore.FilterCapabilities{
			Levels:    []expression.Level{expression.LevelSpan, expression.LevelResource, expression.LevelEvent},
			Operators: []expression.Operator{expression.OpAnd, expression.OpOr, expression.OpNot, expression.OpEq},
		},
		Paginated:   true,
		SpanSearch:  true,
		SpanSorting: true,
	}, nil).Once()

	r, err := http.NewRequest(http.MethodGet, "/api/v3/capabilities", http.NoBody)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	gw.router.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK, w.Code)
	gw.verifySnapshot(t, w.Body.Bytes())
	gw.reader.AssertExpectations(t)
}

func TestHTTPGatewayGetCapabilitiesErrors(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
	}{
		{
			name:   "a reader that cannot report is 501",
			err:    fmt.Errorf("cannot ask: %w", errors.ErrUnsupported),
			status: http.StatusNotImplemented,
		},
		{
			name:   "any other failure is 500",
			err:    assert.AnError,
			status: http.StatusInternalServerError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gw := setupHTTPGatewayNoServer(t, "")
			gw.reader.ExpectedCalls = nil
			gw.reader.On("SearchCapabilities", mock.Anything).Return(tracestore.SearchCapabilities{}, tt.err).Once()

			r, err := http.NewRequest(http.MethodGet, "/api/v3/capabilities", http.NoBody)
			require.NoError(t, err)
			w := httptest.NewRecorder()
			gw.router.ServeHTTP(w, r)
			assert.Equal(t, tt.status, w.Code)
			assert.Contains(t, w.Body.String(), tt.err.Error())
		})
	}
}

func TestHTTPGatewayGetOperationsDefaultSpanKind(t *testing.T) {
	gw := setupHTTPGatewayNoServer(t, "")

	qp := tracestore.OperationQueryParams{ServiceName: "foo"}
	gw.reader.
		On("GetOperations", matchContext, qp).
		Return([]tracestore.Operation{
			{Name: "get_users", SpanKind: "server"},
			{Name: "unspecified_op", SpanKind: ""},
		}, nil).Once()

	r, err := http.NewRequest(http.MethodGet, "/api/v3/operations?service=foo", http.NoBody)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	gw.router.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK, w.Code)

	var response api_v3.GetOperationsResponse
	require.NoError(t, jsonpb.Unmarshal(w.Body, &response))
	require.Len(t, response.Operations, 2)
	assert.Equal(t, "server", response.Operations[0].SpanKind)
	assert.Equal(t, "internal", response.Operations[1].SpanKind, "empty SpanKind should default to 'internal'")
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
	gw.reader.ExpectedCalls = nil

	gw.reader.
		On("FindTraceSummaries", matchContext, qp).
		Return(iter.Seq2[tracestore.PageChunk[[]tracestore.TraceSummary], error](func(yield func(tracestore.PageChunk[[]tracestore.TraceSummary], error) bool) {
			yield(tracestore.PageChunk[[]tracestore.TraceSummary]{
				Results: []tracestore.TraceSummary{{
					RootServiceName:   "frontend",
					RootOperationName: "HTTP GET /",
					SpanCount:         1,
				}},
			}, nil)
			yield(tracestore.PageChunk[[]tracestore.TraceSummary]{
				Results: []tracestore.TraceSummary{{
					RootServiceName:   "backend",
					RootOperationName: "SELECT",
					SpanCount:         2,
				}},
				NextPageToken: "next-page",
			}, nil)
		})).Once()

	r, err := http.NewRequest(http.MethodGet, "/api/v3/trace-summaries?"+q.Encode(), http.NoBody)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	gw.router.ServeHTTP(w, r)

	require.Equal(t, http.StatusOK, w.Code)
	var resp api_v3.FindTraceSummariesResponse
	require.NoError(t, jsonpb.Unmarshal(w.Body, &resp))
	require.Len(t, resp.Summaries, 2)
	assert.Equal(t, "frontend", resp.Summaries[0].RootServiceName)
	assert.Equal(t, "HTTP GET /", resp.Summaries[0].RootOperationName)
	assert.Equal(t, int32(1), resp.Summaries[0].SpanCount)
	assert.Equal(t, "backend", resp.Summaries[1].RootServiceName)
	assert.Equal(t, "SELECT", resp.Summaries[1].RootOperationName)
	assert.Equal(t, int32(2), resp.Summaries[1].SpanCount)
	assert.Equal(t, "next-page", resp.GetNextPageToken())
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

func TestHTTPGatewayFindTraceSummariesInvalidQuery(t *testing.T) {
	gw := setupHTTPGatewayNoServer(t, "")
	r := httptest.NewRequest(http.MethodGet, "/api/v3/trace-summaries", http.NoBody)
	w := httptest.NewRecorder()

	gw.router.ServeHTTP(w, r)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "min and max start time are required")
}

// mockFindSpansQuery builds the query params for a well-formed span search, and the equivalent
// tracestore.SpanQueryParams the reader should be dispatched: the caller sent no pagination, so
// the query service fills in the default page size on the way through.
func mockFindSpansQuery() (url.Values, tracestore.SpanQueryParams) {
	tMin := time.Now().Add(-time.Hour).UTC().Truncate(time.Nanosecond)
	tMax := time.Now().UTC().Truncate(time.Nanosecond)
	q := url.Values{}
	q.Set("query.startTimeMin", tMin.Format(time.RFC3339Nano))
	q.Set("query.startTimeMax", tMax.Format(time.RFC3339Nano))

	return q, tracestore.SpanQueryParams{
		StartTimeMin: tMin,
		StartTimeMax: tMax,
		Pagination:   tracestore.Pagination{PageSize: querysvc.DefaultPageSize},
	}
}

func TestHTTPGatewayFindSpans(t *testing.T) {
	q, qp := mockFindSpansQuery()
	gw := setupHTTPGatewayNoServer(t, "")
	gw.reader.ExpectedCalls = nil
	gw.reader.On("SearchCapabilities", mock.Anything).
		Return(tracestore.SearchCapabilities{SpanSearch: true}, nil).Maybe()
	gw.reader.
		On("FindSpans", matchContext, qp).
		Return(iter.Seq2[tracestore.PageChunk[ptrace.Traces], error](func(yield func(tracestore.PageChunk[ptrace.Traces], error) bool) {
			yield(tracestore.PageChunk[ptrace.Traces]{Results: makeTestTrace()}, nil)
			yield(tracestore.PageChunk[ptrace.Traces]{Results: ptrace.NewTraces(), NextPageToken: "next-page"}, nil)
		})).Once()

	r, err := http.NewRequest(http.MethodGet, "/api/v3/spans?"+q.Encode(), http.NoBody)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	gw.router.ServeHTTP(w, r)

	require.Equal(t, http.StatusOK, w.Code)
	// RFC 0018 §6.1: FindSpans has no proto message typed to carry it as GRPCGatewayWrapper.Result
	// does for the trace endpoints, so the buffered response is wrapped at the JSON level.
	var wrapper struct {
		Result json.RawMessage `json:"result"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &wrapper))
	var resp api_v3.FindSpansResponse
	require.NoError(t, jsonpb.Unmarshal(bytes.NewReader(wrapper.Result), &resp))
	assert.Equal(t, 1, resp.GetSpans().ToTraces().SpanCount())
	assert.Equal(t, "next-page", resp.GetNextPageToken())
}

func TestHTTPGatewayFindSpansError(t *testing.T) {
	q, qp := mockFindSpansQuery()
	gw := setupHTTPGatewayNoServer(t, "")
	gw.reader.ExpectedCalls = nil
	gw.reader.On("SearchCapabilities", mock.Anything).
		Return(tracestore.SearchCapabilities{SpanSearch: true}, nil).Maybe()
	gw.reader.
		On("FindSpans", matchContext, qp).
		Return(iter.Seq2[tracestore.PageChunk[ptrace.Traces], error](func(yield func(tracestore.PageChunk[ptrace.Traces], error) bool) {
			yield(tracestore.PageChunk[ptrace.Traces]{}, assert.AnError)
		})).Once()

	r, err := http.NewRequest(http.MethodGet, "/api/v3/spans?"+q.Encode(), http.NoBody)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	gw.router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), assert.AnError.Error())
}

func TestHTTPGatewayFindSpansUnsupported(t *testing.T) {
	q, _ := mockFindSpansQuery()
	gw := setupHTTPGatewayNoServer(t, "")

	r, err := http.NewRequest(http.MethodGet, "/api/v3/spans?"+q.Encode(), http.NoBody)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	gw.router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusNotImplemented, w.Code)
	assert.Contains(t, w.Body.String(), "does not declare span search support")
}

func TestHTTPGatewayFindSpansInvalidQuery(t *testing.T) {
	gw := setupHTTPGatewayNoServer(t, "")
	r := httptest.NewRequest(http.MethodGet, "/api/v3/spans", http.NoBody)
	w := httptest.NewRecorder()

	gw.router.ServeHTTP(w, r)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "start_time_min and start_time_max are required")
}

// TestHTTPGatewayFindSpansMalformedQuery pins the parser's own 400, as distinct from a
// querysvc-level refusal (TestHTTPGatewayFindSpansInvalidQuery): a query string the parser
// itself cannot read never reaches the query service at all.
func TestHTTPGatewayFindSpansMalformedQuery(t *testing.T) {
	q, _ := mockFindSpansQuery()
	q.Set("query.filter", `{"op":"eq",`)
	gw := setupHTTPGatewayNoServer(t, "")

	r := httptest.NewRequest(http.MethodGet, "/api/v3/spans?"+q.Encode(), http.NoBody)
	w := httptest.NewRecorder()

	gw.router.ServeHTTP(w, r)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "malformed parameter query.filter")
}

// TestHTTPGatewayFindSpansPaginationRefusal pins that a pagination refusal decided in the query
// service (here, the feature gate) reaches the HTTP caller as a 501, since another deployment
// serves the same query, and not as a 500.
func TestHTTPGatewayFindSpansPaginationRefusal(t *testing.T) {
	q, _ := mockFindSpansQuery()
	q.Set("query.pagination.pageToken", "opaque-cursor")
	gw := setupHTTPGatewayNoServer(t, "")

	r, err := http.NewRequest(http.MethodGet, "/api/v3/spans?"+q.Encode(), http.NoBody)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	gw.router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusNotImplemented, w.Code)
	assert.Contains(t, w.Body.String(), querysvc.PaginationGate.ID())
}

func TestTraceIDFromString(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantHi  uint64
		wantLo  uint64
		wantErr bool
	}{
		{
			name:   "hex 64-bit",
			input:  "1",
			wantLo: 1,
		},
		{
			name:   "hex 128-bit",
			input:  "00000000000000010000000000000002",
			wantHi: 1,
			wantLo: 2,
		},
		{
			name:   "base64 with padding (128-bit)",
			input:  "AAAAAAAAAAEAAAAAAAAAAQ==",
			wantHi: 1,
			wantLo: 1,
		},
		{
			name:   "base64 without padding (128-bit)",
			input:  "AAAAAAAAAAEAAAAAAAAAAQ",
			wantHi: 1,
			wantLo: 1,
		},
		{
			name:   "base64 64-bit",
			input:  "AAAAAAAAAAAAAAAAAAAAAQ==",
			wantHi: 0,
			wantLo: 1,
		},
		{
			name:   "base64 with slash",
			input:  "AAAAAAAAAP///////////w==",
			wantHi: 0xFF,
			wantLo: 0xFFFFFFFFFFFFFFFF,
		},
		{
			name:   "base64 with plus",
			input:  "EjRWeJq83vD+3LqYdlQyEA==",
			wantHi: 0x123456789ABCDEF0,
			wantLo: 0xFEDCBA9876543210,
		},
		{
			name:   "url-safe base64 (dash instead of plus)",
			input:  "EjRWeJq83vD-3LqYdlQyEA==",
			wantHi: 0x123456789ABCDEF0,
			wantLo: 0xFEDCBA9876543210,
		},
		{
			name:   "url-safe base64 (underscore instead of slash)",
			input:  "AAAAAAAAAP___________w==",
			wantHi: 0xFF,
			wantLo: 0xFFFFFFFFFFFFFFFF,
		},
		{
			name:    "invalid string",
			input:   "not-a-valid-id!",
			wantErr: true,
		},
		{
			name:    "too long for trace ID",
			input:   "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAlong",
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tid, err := TraceIDFromString(tc.input)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantHi, tid.High)
			assert.Equal(t, tc.wantLo, tid.Low)
		})
	}
}

func TestHTTPGatewayFindSpansOrdering(t *testing.T) {
	const terms = `[{"expression":{"field":{"level":"span","name":"duration"}},"direction":"desc"},{"expression":{"field":{"level":"span","name":"traceID"}}}]`
	for _, supported := range []bool{false, true} {
		t.Run(strconv.FormatBool(supported), func(t *testing.T) {
			q, qp := mockFindSpansQuery()
			gw := setupHTTPGatewayNoServer(t, "")
			gw.reader.ExpectedCalls = nil
			gw.reader.On("SearchCapabilities", mock.Anything).Return(tracestore.SearchCapabilities{SpanSearch: true, SpanSorting: supported}, nil)
			qp.OrderBy = []tracestore.SpanSortOrder{
				{Expression: &expression.FieldRef{Level: expression.LevelSpan, Name: "duration"}, Direction: tracestore.SortDescending},
				{Expression: &expression.FieldRef{Level: expression.LevelSpan, Name: "traceID"}, Direction: tracestore.SortAscending},
			}
			if supported {
				gw.reader.On("FindSpans", matchContext, qp).Return(iter.Seq2[tracestore.PageChunk[ptrace.Traces], error](func(yield func(tracestore.PageChunk[ptrace.Traces], error) bool) {
					for _, service := range []string{"service-a", "service-b", "service-a"} {
						trace := makeTestTrace()
						trace.ResourceSpans().At(0).Resource().Attributes().PutStr("service.name", service)
						yield(tracestore.PageChunk[ptrace.Traces]{Results: trace}, nil)
					}
				})).Once()
			}
			q.Set("query.orderBy", terms)
			r := httptest.NewRequest(http.MethodGet, "/api/v3/spans?"+q.Encode(), http.NoBody)

			w := httptest.NewRecorder()
			gw.router.ServeHTTP(w, r)
			if !supported {
				assert.Equal(t, http.StatusNotImplemented, w.Code)
				assert.Contains(t, w.Body.String(), "does not support explicit span ordering")
				return
			}
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			var wrapper struct {
				Result json.RawMessage `json:"result"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &wrapper))
			var response api_v3.FindSpansResponse
			require.NoError(t, jsonpb.Unmarshal(bytes.NewReader(wrapper.Result), &response))
			var services []string
			for _, resource := range response.Spans.ToTraces().ResourceSpans().All() {
				value, _ := resource.Resource().Attributes().Get("service.name")
				services = append(services, value.Str())
			}
			assert.Equal(t, []string{"service-a", "service-b", "service-a"}, services)
			gw.reader.AssertExpectations(t)
		})
	}
}

func TestHTTPGatewayFindSpansMalformedOrder(t *testing.T) {
	for _, tc := range []struct {
		raw, want string
	}{
		{`[`, "malformed parameter query.orderBy"},
		{`[null]`, "malformed parameter query.orderBy"},
		{`[{}]`, "malformed parameter query.orderBy"},
		// Valid JSON that is not a list of sort terms fails protobuf decoding rather than json.Valid.
		{`{"not":"an array"}`, "malformed parameter query.orderBy"},
		{`[{"bogus":true}]`, "malformed parameter query.orderBy"},
		// Content after the array would otherwise be spliced into the wrapper object unnoticed.
		{`[]}{"garbage":true}`, "not a single JSON value"},
		{`[{"expression":{"field":{"level":"span","name":"duration"}}}],"orderBy":[]`, "not a single JSON value"},
		{`[{"expression":{"field":{"level":"span","name":"duration"}},"direction":"sideways"}]`, `direction \"sideways\" is unsupported`},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			q, _ := mockFindSpansQuery()
			q.Set("query.orderBy", tc.raw)
			gw := setupHTTPGatewayNoServer(t, "")
			gw.reader.ExpectedCalls = nil
			gw.reader.On("SearchCapabilities", mock.Anything).Return(tracestore.SearchCapabilities{SpanSearch: true, SpanSorting: true}, nil)
			w := httptest.NewRecorder()
			gw.router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v3/spans?"+q.Encode(), http.NoBody))
			assert.Equal(t, http.StatusBadRequest, w.Code)
			assert.Contains(t, w.Body.String(), tc.want)
			gw.reader.AssertNotCalled(t, "FindSpans", mock.Anything, mock.Anything)
		})
	}
}
