// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package apiv3

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/jtracer"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	spanstoremocks "github.com/jaegertracing/jaeger/storage/spanstore/mocks"
	dependencyStoreMocks "github.com/jaegertracing/jaeger/storage_v2/depstore/mocks"
	"github.com/jaegertracing/jaeger/storage_v2/v1adapter"
)

func setupHTTPGatewayNoServer(
	_ *testing.T,
	basePath string,
) *testGateway {
	gw := &testGateway{
		reader: &spanstoremocks.Reader{},
	}

	q := querysvc.NewQueryService(v1adapter.NewTraceReader(gw.reader),
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
	traceId, _ := model.TraceIDFromString("123")
	testCases := []struct {
		name          string
		expectedQuery spanstore.GetTraceParameters
		params        map[string]string
	}{
		{
			"TestGetTrace",
			spanstore.GetTraceParameters{
				TraceID: traceId,
			},
			map[string]string{},
		},
		{
			"TestGetTraceWithTimeWindow",
			spanstore.GetTraceParameters{
				TraceID:   traceId,
				StartTime: time.Date(2000, time.January, 0o2, 12, 30, 8, 999999998, time.UTC),
				EndTime:   time.Date(2000, time.April, 0o5, 13, 55, 16, 999999992, time.UTC),
			},
			map[string]string{
				"start_time": "2000-01-02T12:30:08.999999998Z",
				"end_time":   "2000-04-05T21:55:16.999999992+08:00",
			},
		},
	}

	testUri := "/api/v3/traces/123"

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gw := setupHTTPGatewayNoServer(t, "")
			gw.reader.
				On("GetTrace", matchContext, tc.expectedQuery).
				Return(&model.Trace{}, nil).Once()

			q := url.Values{}
			for k, v := range tc.params {
				q.Set(k, v)
			}
			testUrl := testUri
			if len(tc.params) > 0 {
				testUrl += "?" + q.Encode()
			}

			r, err := http.NewRequest(http.MethodGet, testUrl, nil)
			require.NoError(t, err)
			w := httptest.NewRecorder()
			gw.router.ServeHTTP(w, r)
			gw.reader.AssertCalled(t, "GetTrace", matchContext, tc.expectedQuery)
		})
	}
}

func TestHTTPGatewayGetTraceMalformedInputErrors(t *testing.T) {
	testCases := []struct {
		name          string
		requestUrl    string
		expectedError string
	}{
		{
			"TestGetTrace",
			"/api/v3/traces/xyz",
			"malformed parameter trace_id",
		},
		{
			"TestGetTraceWithInvalidStartTime",
			"/api/v3/traces/123?start_time=abc",
			"malformed parameter start_time",
		},
		{
			"TestGetTraceWithInvalidEndTime",
			"/api/v3/traces/123?end_time=xyz",
			"malformed parameter end_time",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gw := setupHTTPGatewayNoServer(t, "")
			gw.reader.
				On("GetTrace", matchContext, matchGetTraceParameters).
				Return(&model.Trace{}, nil).Once()

			r, err := http.NewRequest(http.MethodGet, tc.requestUrl, nil)
			require.NoError(t, err)
			w := httptest.NewRecorder()
			gw.router.ServeHTTP(w, r)
			assert.Contains(t, w.Body.String(), tc.expectedError)
		})
	}
}

func TestHTTPGatewayGetTraceInternalErrors(t *testing.T) {
	gw := setupHTTPGatewayNoServer(t, "")
	// error from span reader
	const simErr = "simulated error"
	gw.reader.
		On("GetTrace", matchContext, matchGetTraceParameters).
		Return(nil, errors.New(simErr)).Once()

	r, err := http.NewRequest(http.MethodGet, "/api/v3/traces/123", nil)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	gw.router.ServeHTTP(w, r)
	assert.Contains(t, w.Body.String(), simErr)
}

func mockFindQueries() (url.Values, *spanstore.TraceQueryParameters) {
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

	return q, &spanstore.TraceQueryParameters{
		ServiceName:   "foo",
		OperationName: "bar",
		StartTimeMin:  time1,
		StartTimeMax:  time2,
		DurationMin:   1 * time.Second,
		DurationMax:   2 * time.Second,
		NumTraces:     10,
	}
}

func TestHTTPGatewayFindTracesErrors(t *testing.T) {
	goodTimeV := time.Now()
	goodTime := goodTimeV.Format(time.RFC3339Nano)
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
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			q := url.Values{}
			for k, v := range tc.params {
				q.Set(k, v)
			}
			r, err := http.NewRequest(http.MethodGet, "/api/v3/traces?"+q.Encode(), nil)
			require.NoError(t, err)
			w := httptest.NewRecorder()

			gw := setupHTTPGatewayNoServer(t, "")
			gw.router.ServeHTTP(w, r)
			assert.Contains(t, w.Body.String(), tc.expErr)
		})
	}
	t.Run("span reader error", func(t *testing.T) {
		q, qp := mockFindQueries()
		const simErr = "simulated error"
		r, err := http.NewRequest(http.MethodGet, "/api/v3/traces?"+q.Encode(), nil)
		require.NoError(t, err)
		w := httptest.NewRecorder()

		gw := setupHTTPGatewayNoServer(t, "")
		gw.reader.
			On("FindTraces", matchContext, qp).
			Return(nil, errors.New(simErr)).Once()

		gw.router.ServeHTTP(w, r)
		assert.Contains(t, w.Body.String(), simErr)
	})
}

func TestHTTPGatewayGetServicesErrors(t *testing.T) {
	gw := setupHTTPGatewayNoServer(t, "")

	const simErr = "simulated error"
	gw.reader.
		On("GetServices", matchContext).
		Return(nil, errors.New(simErr)).Once()

	r, err := http.NewRequest(http.MethodGet, "/api/v3/services", nil)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	gw.router.ServeHTTP(w, r)
	assert.Contains(t, w.Body.String(), simErr)
}

func TestHTTPGatewayGetOperationsErrors(t *testing.T) {
	gw := setupHTTPGatewayNoServer(t, "")

	qp := spanstore.OperationQueryParameters{ServiceName: "foo", SpanKind: "server"}
	const simErr = "simulated error"
	gw.reader.
		On("GetOperations", matchContext, qp).
		Return(nil, errors.New(simErr)).Once()

	r, err := http.NewRequest(http.MethodGet, "/api/v3/operations?service=foo&span_kind=server", nil)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	gw.router.ServeHTTP(w, r)
	assert.Contains(t, w.Body.String(), simErr)
}
