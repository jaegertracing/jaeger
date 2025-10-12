// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package apiv3

import (
	"errors"
	"fmt"
	"iter"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc/v2/querysvc"
	"github.com/jaegertracing/jaeger/internal/jtracer"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
	dependencyStoreMocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore/mocks"
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

	q := querysvc.NewQueryService(gw.reader,
		&dependencyStoreMocks.Reader{},
		querysvc.QueryServiceOptions{},
	)

	hgw := &HTTPGateway{
		QueryService: q,
		Logger:       zap.NewNop(),
		Tracer:       jtracer.NoOp().OTEL,
	}

	gw.router = &mux.Router{}
	if basePath != "" && basePath != "/" {
		gw.router = gw.router.PathPrefix(basePath).Subrouter()
	}
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
			name:   "TestGetTrace",
			params: map[string]string{},
			expectedQuery: tracestore.GetTraceParams{
				TraceID: traceID,
			},
		},
		{
			name: "TestGetTraceWithTimeWindow",
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

func TestHTTPGatewayGetTraceMalformedInputErrors(t *testing.T) {
	testCases := []struct {
		name          string
		requestUrl    string
		expectedError string
	}{
		{
			name:          "TestGetTrace",
			requestUrl:    "/api/v3/traces/xyz",
			expectedError: "malformed parameter trace_id",
		},
		{
			name:          "TestGetTraceWithInvalidStartTime",
			requestUrl:    "/api/v3/traces/1?start_time=abc",
			expectedError: "malformed parameter start_time",
		},
		{
			name:          "TestGetTraceWithInvalidEndTime",
			requestUrl:    "/api/v3/traces/1?end_time=xyz",
			expectedError: "malformed parameter end_time",
		},
		{
			name:          "TestGetTraceWithInvalidRawTraces",
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
	// mock performs deep comparison of the timestamps and can fail
	// if they are different in the timezone or the monotonic clocks.
	// To void that we truncate monotonic clock and force UTC timezone.
	time1 := time.Now().UTC().Truncate(time.Nanosecond)
	time2 := time1.Add(-time.Second).UTC().Truncate(time.Nanosecond)
	q := url.Values{}
	q.Set(paramServiceName, "foo")
	q.Set(paramOperationName, "bar")
	q.Set(paramTimeMin, time1.Format(time.RFC3339Nano))
	q.Set(paramTimeMax, time2.Format(time.RFC3339Nano))
	q.Set(paramDurationMin, "1s")
	q.Set(paramDurationMax, "2s")
	q.Set(paramNumTraces, "10")

	return q, tracestore.TraceQueryParams{
		ServiceName:   "foo",
		OperationName: "bar",
		Attributes:    pcommon.NewMap(),
		StartTimeMin:  time1,
		StartTimeMax:  time2,
		DurationMin:   1 * time.Second,
		DurationMax:   2 * time.Second,
		SearchDepth:   10,
	}
}

func TestHTTPGatewayFindTracesErrors(t *testing.T) {
	goodTimeV := time.Now()
	goodTime := goodTimeV.Format(time.RFC3339Nano)
	goodDuration := "1s"
	timeRangeErr := fmt.Sprintf("%s and %s are required", paramTimeMin, paramTimeMax)
	testCases := []struct {
		name   string
		params map[string]string
		expErr string
	}{
		{
			name:   "no time range",
			expErr: timeRangeErr,
		},
		{
			name:   "no max time",
			params: map[string]string{paramTimeMin: goodTime},
			expErr: timeRangeErr,
		},
		{
			name:   "no min time",
			params: map[string]string{paramTimeMax: goodTime},
			expErr: timeRangeErr,
		},
		{
			name:   "bax min time",
			params: map[string]string{paramTimeMin: "NaN", paramTimeMax: goodTime},
			expErr: paramTimeMin,
		},
		{
			name:   "bax max time",
			params: map[string]string{paramTimeMin: goodTime, paramTimeMax: "NaN"},
			expErr: paramTimeMax,
		},
		{
			name:   "bad num_traces",
			params: map[string]string{paramTimeMin: goodTime, paramTimeMax: goodTime, paramNumTraces: "NaN"},
			expErr: paramNumTraces,
		},
		{
			name:   "bad min duration",
			params: map[string]string{paramTimeMin: goodTime, paramTimeMax: goodTime, paramDurationMin: "NaN"},
			expErr: paramDurationMin,
		},
		{
			name:   "bad max duration",
			params: map[string]string{paramTimeMin: goodTime, paramTimeMax: goodTime, paramDurationMax: "NaN"},
			expErr: paramDurationMax,
		},
		{
			name: "bad raw traces",
			params: map[string]string{
				paramTimeMin:        goodTime,
				paramTimeMax:        goodTime,
				paramDurationMax:    goodDuration,
				paramQueryRawTraces: "foobar",
			},
			expErr: paramQueryRawTraces,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			q := url.Values{}
			for k, v := range tc.params {
				q.Set(k, v)
			}
			r, err := http.NewRequest(http.MethodGet, "/api/v3/traces?"+q.Encode(), http.NoBody)
			require.NoError(t, err)
			w := httptest.NewRecorder()

			gw := setupHTTPGatewayNoServer(t, "")
			gw.router.ServeHTTP(w, r)
			assert.Contains(t, w.Body.String(), tc.expErr)
		})
	}
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
		Return(nil, assert.AnError).Once()

	r, err := http.NewRequest(http.MethodGet, "/api/v3/operations?service=foo&span_kind=server", http.NoBody)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	gw.router.ServeHTTP(w, r)
	assert.Contains(t, w.Body.String(), assert.AnError.Error())
}

// TestHTTPGatewayStreamingResponse verifies that chunked encoding is used for streaming responses
func TestHTTPGatewayStreamingResponse(t *testing.T) {
	gw := setupHTTPGatewayNoServer(t, "")

	// Create multiple trace batches to verify streaming
	trace1 := makeTestTrace()
	trace2 := ptrace.NewTraces()
	resources := trace2.ResourceSpans().AppendEmpty()
	scopes := resources.ScopeSpans().AppendEmpty()
	spanB := scopes.Spans().AppendEmpty()
	spanB.SetName("second-span")
	spanB.SetTraceID(traceID)
	spanB.SetSpanID(pcommon.SpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 3}))

	// Setup iterator that returns multiple batches
	gw.reader.
		On("GetTraces", matchContext, mock.AnythingOfType("[]tracestore.GetTraceParams")).
		Return(iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
			// Yield first batch
			if !yield([]ptrace.Traces{trace1}, nil) {
				return
			}
			// Yield second batch
			yield([]ptrace.Traces{trace2}, nil)
		})).Once()

	r, err := http.NewRequest(http.MethodGet, "/api/v3/traces/1", http.NoBody)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	gw.router.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)

	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	assert.Equal(t, "identity", w.Header().Get("Content-Encoding"))

	body := w.Body.String()
	assert.Contains(t, body, "foobar")      // First trace span name
	assert.Contains(t, body, "second-span") // Second trace span name
}

// TestHTTPGatewayStreamingMultipleBatches tests streaming with multiple trace batches
func TestHTTPGatewayStreamingMultipleBatches(t *testing.T) {
	gw := setupHTTPGatewayNoServer(t, "")

	trace1 := makeTestTrace()
	trace2 := ptrace.NewTraces()
	resources := trace2.ResourceSpans().AppendEmpty()
	scopes := resources.ScopeSpans().AppendEmpty()
	spanB := scopes.Spans().AppendEmpty()
	spanB.SetName("batch2-span")
	spanB.SetTraceID(traceID)
	spanB.SetSpanID(pcommon.SpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 4}))

	gw.reader.
		On("GetTraces", matchContext, mock.AnythingOfType("[]tracestore.GetTraceParams")).
		Return(iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
			if !yield([]ptrace.Traces{trace1}, nil) {
				return
			}
			if !yield([]ptrace.Traces{trace2}, nil) {
				return
			}
		})).Once()

	r, err := http.NewRequest(http.MethodGet, "/api/v3/traces/1", http.NoBody)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	gw.router.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)

	body := w.Body.String()
	assert.Contains(t, body, "foobar")
	assert.Contains(t, body, "batch2-span")
}

func TestHTTPGatewayStreamingFallbackNoFlusher(t *testing.T) {
	gw := setupHTTPGatewayNoServer(t, "")

	trace1 := makeTestTrace()
	gw.reader.
		On("GetTraces", matchContext, mock.AnythingOfType("[]tracestore.GetTraceParams")).
		Return(iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
			yield([]ptrace.Traces{trace1}, nil)
		})).Once()

	r, err := http.NewRequest(http.MethodGet, "/api/v3/traces/1", http.NoBody)
	require.NoError(t, err)

	// Use a custom ResponseWriter that doesn't implement http.Flusher
	w := &nonFlushableRecorder{ResponseWriter: httptest.NewRecorder()}
	gw.router.ServeHTTP(w, r)

	// Should still work but fall back to buffered response
	assert.Equal(t, http.StatusOK, w.ResponseWriter.(*httptest.ResponseRecorder).Code)
	assert.Contains(t, w.ResponseWriter.(*httptest.ResponseRecorder).Body.String(), "foobar")
}

// TestHTTPGatewayStreamingFallbackMultipleTraces tests fallback with multiple traces
func TestHTTPGatewayStreamingFallbackMultipleTraces(t *testing.T) {
	gw := setupHTTPGatewayNoServer(t, "")

	trace1 := makeTestTrace()
	trace2 := ptrace.NewTraces()
	resources := trace2.ResourceSpans().AppendEmpty()
	scopes := resources.ScopeSpans().AppendEmpty()
	spanB := scopes.Spans().AppendEmpty()
	spanB.SetName("fallback-span")
	spanB.SetTraceID(traceID)
	spanB.SetSpanID(pcommon.SpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 6}))

	gw.reader.
		On("GetTraces", matchContext, mock.AnythingOfType("[]tracestore.GetTraceParams")).
		Return(iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
			// Multiple traces to test the combining logic in returnTraces
			if !yield([]ptrace.Traces{trace1}, nil) {
				return
			}
			yield([]ptrace.Traces{trace2}, nil)
		})).Once()

	r, err := http.NewRequest(http.MethodGet, "/api/v3/traces/1", http.NoBody)
	require.NoError(t, err)

	// Use a non-flushable writer to trigger fallback path
	w := &nonFlushableRecorder{ResponseWriter: httptest.NewRecorder()}
	gw.router.ServeHTTP(w, r)

	// Should combine multiple traces and return them
	assert.Equal(t, http.StatusOK, w.ResponseWriter.(*httptest.ResponseRecorder).Code)
	body := w.ResponseWriter.(*httptest.ResponseRecorder).Body.String()
	assert.Contains(t, body, "foobar")
	assert.Contains(t, body, "fallback-span")
}

type nonFlushableRecorder struct {
	http.ResponseWriter
}

func (n *nonFlushableRecorder) Header() http.Header {
	return n.ResponseWriter.Header()
}

func (n *nonFlushableRecorder) Write(b []byte) (int, error) {
	return n.ResponseWriter.Write(b)
}

func (n *nonFlushableRecorder) WriteHeader(statusCode int) {
	n.ResponseWriter.WriteHeader(statusCode)
}

// TestHTTPGatewayStreamingWithEmptyBatches tests handling of empty trace batches
func TestHTTPGatewayStreamingWithEmptyBatches(t *testing.T) {
	gw := setupHTTPGatewayNoServer(t, "")

	trace1 := makeTestTrace()

	// Setup iterator that returns empty batches mixed with real data
	gw.reader.
		On("GetTraces", matchContext, mock.AnythingOfType("[]tracestore.GetTraceParams")).
		Return(iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
			// Yield empty batch (should be skipped)
			if !yield([]ptrace.Traces{}, nil) {
				return
			}
			// Yield real batch
			if !yield([]ptrace.Traces{trace1}, nil) {
				return
			}
			// Another empty batch
			yield([]ptrace.Traces{}, nil)
		})).Once()

	r, err := http.NewRequest(http.MethodGet, "/api/v3/traces/1", http.NoBody)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	gw.router.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "foobar")
}

// TestHTTPGatewayStreamingNoTracesFound tests 404 when no traces exist
func TestHTTPGatewayStreamingNoTracesFound(t *testing.T) {
	gw := setupHTTPGatewayNoServer(t, "")

	// Setup iterator that returns only empty batches
	gw.reader.
		On("GetTraces", matchContext, mock.AnythingOfType("[]tracestore.GetTraceParams")).
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

// TestHTTPGatewayFindTracesStreaming tests findTraces endpoint with streaming
func TestHTTPGatewayFindTracesStreaming(t *testing.T) {
	q, qp := mockFindQueries()
	trace1 := makeTestTrace()

	gw := setupHTTPGatewayNoServer(t, "")
	gw.reader.
		On("FindTraces", matchContext, qp).
		Return(iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
			yield([]ptrace.Traces{trace1}, nil)
		})).Once()

	r, err := http.NewRequest(http.MethodGet, "/api/v3/traces?"+q.Encode(), http.NoBody)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	gw.router.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "foobar")
}

// TestHTTPGatewayStreamingMarshalError tests handling of marshal errors during streaming
func TestHTTPGatewayStreamingMarshalError(t *testing.T) {
	gw := setupHTTPGatewayNoServer(t, "")

	trace1 := makeTestTrace()

	gw.reader.
		On("GetTraces", matchContext, mock.AnythingOfType("[]tracestore.GetTraceParams")).
		Return(iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
			yield([]ptrace.Traces{trace1}, nil)
		})).Once()

	r, err := http.NewRequest(http.MethodGet, "/api/v3/traces/1", http.NoBody)
	require.NoError(t, err)

	// Use a logger to capture error logs
	logger, log := testutils.NewLogger()
	q := querysvc.NewQueryService(gw.reader,
		&dependencyStoreMocks.Reader{},
		querysvc.QueryServiceOptions{},
	)
	hgw := &HTTPGateway{
		QueryService: q,
		Logger:       logger,
		Tracer:       jtracer.NoOp().OTEL,
	}
	gw.router = &mux.Router{}
	hgw.RegisterRoutes(gw.router)

	// Use a ResponseWriter that fails immediately
	w := &failingWriter{
		ResponseRecorder: httptest.NewRecorder(),
		failImmediately:  true,
	}
	gw.router.ServeHTTP(w, r)

	// Should log the marshal error
	assert.Contains(t, log.String(), "Failed to marshal trace chunk")
}

// failingWriter is a ResponseWriter that simulates write failures
type failingWriter struct {
	*httptest.ResponseRecorder
	writeCount      int
	failAfterBytes  int
	failImmediately bool
}

func (f *failingWriter) Write(b []byte) (int, error) {
	f.writeCount++
	if f.failImmediately && f.writeCount == 1 {
		// Fail on first write (marshal output)
		return 0, assert.AnError
	}
	if f.failAfterBytes > 0 && f.writeCount == 2 {
		// Fail on second write (separator)
		return 0, assert.AnError
	}
	return f.ResponseRecorder.Write(b)
}

func (*failingWriter) Flush() {
	// Implement Flusher interface
}

// TestHTTPGatewayStreamingFirstChunkWrite tests various edge cases in first chunk handling
func TestHTTPGatewayStreamingFirstChunkWrite(t *testing.T) {
	gw := setupHTTPGatewayNoServer(t, "")

	trace1 := makeTestTrace()
	trace2 := ptrace.NewTraces()
	resources := trace2.ResourceSpans().AppendEmpty()
	scopes := resources.ScopeSpans().AppendEmpty()
	spanB := scopes.Spans().AppendEmpty()
	spanB.SetName("span2")
	spanB.SetTraceID(traceID)
	spanB.SetSpanID(pcommon.SpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 5}))

	gw.reader.
		On("GetTraces", matchContext, mock.AnythingOfType("[]tracestore.GetTraceParams")).
		Return(iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
			// Multiple traces to ensure we test the iteration logic
			yield([]ptrace.Traces{trace1, trace2}, nil)
		})).Once()

	r, err := http.NewRequest(http.MethodGet, "/api/v3/traces/1", http.NoBody)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	gw.router.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "foobar")
	assert.Contains(t, w.Body.String(), "span2")
}

// TestHTTPGatewayStreamingErrorBeforeFirstChunk tests error handling before streaming starts
func TestHTTPGatewayStreamingErrorBeforeFirstChunk(t *testing.T) {
	gw := setupHTTPGatewayNoServer(t, "")

	// Setup iterator that returns error immediately
	gw.reader.
		On("GetTraces", matchContext, mock.AnythingOfType("[]tracestore.GetTraceParams")).
		Return(iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
			yield(nil, assert.AnError)
		})).Once()

	r, err := http.NewRequest(http.MethodGet, "/api/v3/traces/1", http.NoBody)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	gw.router.ServeHTTP(w, r)

	// Should return 500 error since streaming hasn't started
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), assert.AnError.Error())
}

// TestHTTPGatewayStreamingFallbackError tests fallback path with errors
func TestHTTPGatewayStreamingFallbackError(t *testing.T) {
	gw := setupHTTPGatewayNoServer(t, "")

	// Mock reader to return error
	gw.reader.
		On("GetTraces", matchContext, mock.AnythingOfType("[]tracestore.GetTraceParams")).
		Return(iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
			yield(nil, assert.AnError)
		})).Once()

	r, err := http.NewRequest(http.MethodGet, "/api/v3/traces/1", http.NoBody)
	require.NoError(t, err)

	// Use a non-flushing writer to trigger fallback
	w := &nonFlushingWriter{ResponseRecorder: httptest.NewRecorder()}
	gw.router.ServeHTTP(w, r)

	// Should return 500 error via returnTraces fallback path
	assert.Equal(t, http.StatusInternalServerError, w.ResponseRecorder.Code)
}

// nonFlushingWriter simulates a ResponseWriter without Flusher interface
type nonFlushingWriter struct {
	*httptest.ResponseRecorder
}

// TestHTTPGatewayStreamingFallbackNoTraces tests fallback path with no traces (404)
func TestHTTPGatewayStreamingFallbackNoTraces(t *testing.T) {
	gw := setupHTTPGatewayNoServer(t, "")

	// Mock reader to return empty traces
	gw.reader.
		On("GetTraces", matchContext, mock.AnythingOfType("[]tracestore.GetTraceParams")).
		Return(iter.Seq2[[]ptrace.Traces, error](func(_ func([]ptrace.Traces, error) bool) {
			// Return empty - no traces found
		})).Once()

	r, err := http.NewRequest(http.MethodGet, "/api/v3/traces/1", http.NoBody)
	require.NoError(t, err)

	// Use a non-flushing writer to trigger fallback path
	w := &nonFlushingWriter{ResponseRecorder: httptest.NewRecorder()}
	gw.router.ServeHTTP(w, r)

	// Should return 404 via returnTraces fallback path
	assert.Equal(t, http.StatusNotFound, w.ResponseRecorder.Code)
	assert.Contains(t, w.ResponseRecorder.Body.String(), "No traces found")
}
