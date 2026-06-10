// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package apiv3

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/proto/api_v3"
	depsmocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore/mocks"
	tracemocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore/mocks"
)

func newTestGateway(t *testing.T) (*HTTPGateway, *depsmocks.Reader, *http.ServeMux) {
	t.Helper()
	traceReader := &tracemocks.Reader{}
	depReader := &depsmocks.Reader{}

	querySvc := querysvc.NewQueryService(
		traceReader,
		depReader,
		querysvc.QueryServiceOptions{},
	)

	gateway := &HTTPGateway{
		QueryService: querySvc,
		Logger:       zap.NewNop(),
		Tracer:       noop.NewTracerProvider(),
	}

	router := http.NewServeMux()
	gateway.RegisterRoutes(router)

	return gateway, depReader, router
}

func TestHTTPGatewayGetDependencies(t *testing.T) {
	_, depReader, router := newTestGateway(t)

	now := time.Now().UTC()
	startTime := now.Add(-24 * time.Hour)
	endTime := now

	expectedDeps := []model.DependencyLink{
		{
			Parent:    "frontend",
			Child:     "backend",
			CallCount: 100,
			Source:    "traces",
		},
		{
			Parent:    "backend",
			Child:     "database",
			CallCount: 500,
			Source:    "traces",
		},
	}

	depReader.On("GetDependencies", mock.Anything, mock.Anything).Return(expectedDeps, nil).Once()

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v3/dependencies?start_time="+startTime.Format(time.RFC3339Nano)+"&end_time="+endTime.Format(time.RFC3339Nano),
		nil,
	)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "Response body: %s", w.Body.String())
	var resp api_v3.DependenciesResponse
	require.NoError(t, jsonpb.Unmarshal(w.Body, &resp))
	require.Len(t, resp.Dependencies, 2)
	assert.Equal(t, "frontend", resp.Dependencies[0].Parent)
	assert.Equal(t, "backend", resp.Dependencies[1].Parent)
	depReader.AssertExpectations(t)
}

func TestHTTPGatewayGetDependenciesErrors(t *testing.T) {
	_, _, router := newTestGateway(t)

	goodStartTime := "2026-01-24T12:00:00Z"
	goodEndTime := "2026-01-24T16:00:00Z"

	tests := []struct {
		name           string
		url            string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "missing start_time and end_time",
			url:            "/api/v3/dependencies",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "startTime",
		},
		{
			name:           "missing start_time",
			url:            "/api/v3/dependencies?end_time=" + goodEndTime,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "startTime",
		},
		{
			name:           "missing end_time",
			url:            "/api/v3/dependencies?start_time=" + goodStartTime,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "endTime",
		},
		{
			name:           "invalid start_time",
			url:            "/api/v3/dependencies?start_time=invalid&end_time=" + goodEndTime,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "startTime",
		},
		{
			name:           "invalid end_time",
			url:            "/api/v3/dependencies?start_time=" + goodStartTime + "&end_time=invalid",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "endTime",
		},
		{
			name:           "end_time not after start_time",
			url:            "/api/v3/dependencies?start_time=" + goodEndTime + "&end_time=" + goodStartTime,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "endTime",
		},
		{
			name:           "equal start_time and end_time",
			url:            "/api/v3/dependencies?start_time=" + goodStartTime + "&end_time=" + goodStartTime,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "endTime",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
			assert.Contains(t, w.Body.String(), tt.expectedBody)
		})
	}
}

func TestHTTPGatewayGetDependencies_StorageError(t *testing.T) {
	_, depReader, router := newTestGateway(t)

	depReader.On("GetDependencies", mock.Anything, mock.Anything).Return(nil, assert.AnError).Once()

	now := time.Now().UTC()
	startTime := now.Add(-24 * time.Hour)
	endTime := now
	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v3/dependencies?start_time="+startTime.Format(time.RFC3339Nano)+"&end_time="+endTime.Format(time.RFC3339Nano),
		nil,
	)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	depReader.AssertExpectations(t)
}

func TestHTTPGatewayGetDependencies_EmptyResponse(t *testing.T) {
	_, depReader, router := newTestGateway(t)

	depReader.On("GetDependencies", mock.Anything, mock.Anything).Return([]model.DependencyLink{}, nil).Once()

	now := time.Now().UTC()
	startTime := now.Add(-24 * time.Hour)
	endTime := now
	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v3/dependencies?start_time="+startTime.Format(time.RFC3339Nano)+"&end_time="+endTime.Format(time.RFC3339Nano),
		nil,
	)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp api_v3.DependenciesResponse
	require.NoError(t, jsonpb.Unmarshal(w.Body, &resp))
	assert.Empty(t, resp.Dependencies)
	depReader.AssertExpectations(t)
}
