// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package apiv3

import (
	"iter"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

func TestHTTPGateway_DeprecationHeader_whenDeprecatedParamUsed(t *testing.T) {
	gw := setupHTTPGatewayNoServer(t, "")
	gw.reader.
		On("GetTraces", matchContext, mock.AnythingOfType("[]tracestore.GetTraceParams")).
		Return(iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
			yield([]ptrace.Traces{makeTestTrace()}, nil)
		})).Once()

	r, err := http.NewRequest(http.MethodGet, "/api/v3/traces/1?"+paramStartTimeDeprecated+"=2000-01-02T12:30:08Z", http.NoBody)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	gw.router.ServeHTTP(w, r)

	assert.Equal(t, "true", w.Header().Get("Deprecation"))
	assert.Equal(t, paramStartTimeDeprecated, w.Header().Get("Deprecated-Params"))
}

func TestHTTPGateway_DeprecationHeader_absentWhenCanonicalOnly(t *testing.T) {
	gw := setupHTTPGatewayNoServer(t, "")
	gw.reader.
		On("GetTraces", matchContext, mock.AnythingOfType("[]tracestore.GetTraceParams")).
		Return(iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
			yield([]ptrace.Traces{makeTestTrace()}, nil)
		})).Once()

	r, err := http.NewRequest(http.MethodGet, "/api/v3/traces/1?"+paramStartTime+"=2000-01-02T12:30:08Z", http.NoBody)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	gw.router.ServeHTTP(w, r)

	assert.Empty(t, w.Header().Get("Deprecation"))
}

func TestHTTPGateway_DeprecationHeader_absentWhenCanonicalWinsOverDeprecated(t *testing.T) {
	gw := setupHTTPGatewayNoServer(t, "")
	gw.reader.
		On("GetTraces", matchContext, mock.AnythingOfType("[]tracestore.GetTraceParams")).
		Return(iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
			yield([]ptrace.Traces{makeTestTrace()}, nil)
		})).Once()

	q := url.Values{}
	q.Set(paramStartTime, "2000-01-02T12:30:08Z")
	q.Set(paramStartTimeDeprecated, "1999-01-01T00:00:00Z")
	r, err := http.NewRequest(http.MethodGet, "/api/v3/traces/1?"+q.Encode(), http.NoBody)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	gw.router.ServeHTTP(w, r)

	assert.Empty(t, w.Header().Get("Deprecation"))
}

func TestHTTPGateway_rawTracesBoolVariants(t *testing.T) {
	testCases := []struct {
		name  string
		value string
	}{
		{name: "true", value: "true"},
		{name: "TRUE", value: "TRUE"},
		{name: "1", value: "1"},
		{name: "false", value: "false"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gw := setupHTTPGatewayNoServer(t, "")
			gw.reader.
				On("GetTraces", matchContext, mock.MatchedBy(func(params []tracestore.GetTraceParams) bool {
					return len(params) == 1
				})).
				Return(iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
					yield([]ptrace.Traces{makeTestTrace()}, nil)
				})).Once()

			r, err := http.NewRequest(http.MethodGet, "/api/v3/traces/1?"+paramRawTraces+"="+tc.value, http.NoBody)
			require.NoError(t, err)
			w := httptest.NewRecorder()
			gw.router.ServeHTTP(w, r)
			assert.Equal(t, http.StatusOK, w.Code)
		})
	}
}

func TestHTTPGateway_findTraces_deprecatedFallback(t *testing.T) {
	time1 := time.Date(2020, 1, 2, 12, 0, 0, 0, time.UTC)
	time2 := time.Date(2020, 1, 2, 13, 0, 0, 0, time.UTC)
	q := url.Values{}
	q.Set(paramServiceNameDeprecated, "svc")
	q.Set(paramOperationNameDeprecated, "op")
	q.Set(paramTimeMinDeprecated, time1.Format(time.RFC3339Nano))
	q.Set(paramTimeMaxDeprecated, time2.Format(time.RFC3339Nano))
	q.Set(paramNumTracesDeprecated, "3")

	gw := setupHTTPGatewayNoServer(t, "")
	gw.reader.
		On("FindTraces", matchContext, mock.MatchedBy(func(qp tracestore.TraceQueryParams) bool {
			return qp.ServiceName == "svc" &&
				qp.OperationName == "op" &&
				qp.StartTimeMin.Equal(time1) &&
				qp.StartTimeMax.Equal(time2) &&
				qp.SearchDepth == 3
		})).
		Return(iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
			yield([]ptrace.Traces{makeTestTrace()}, nil)
		})).Once()

	r, err := http.NewRequest(http.MethodGet, "/api/v3/traces?"+q.Encode(), http.NoBody)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	gw.router.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "true", w.Header().Get("Deprecation"))
}
