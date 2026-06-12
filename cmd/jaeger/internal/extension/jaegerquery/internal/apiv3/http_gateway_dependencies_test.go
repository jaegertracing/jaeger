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
	now := time.Now().UTC()
	startTime := now.Add(-24 * time.Hour)
	endTime := now

	tests := []struct {
		name         string
		dependencies []model.DependencyLink
		expectedResp *api_v3.DependenciesResponse
	}{
		{
			name: "with data",
			dependencies: []model.DependencyLink{
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
			},
			expectedResp: &api_v3.DependenciesResponse{
				Dependencies: []*api_v3.Dependency{
					{
						Parent:    "frontend",
						Child:     "backend",
						CallCount: 100,
					},
					{
						Parent:    "backend",
						Child:     "database",
						CallCount: 500,
					},
				},
			},
		},
		{
			name:         "empty response",
			dependencies: []model.DependencyLink{},
			expectedResp: &api_v3.DependenciesResponse{
				Dependencies: []*api_v3.Dependency{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, depReader, router := newTestGateway(t)
			depReader.On("GetDependencies", mock.Anything, mock.Anything).Return(tt.dependencies, nil).Once()

			req := httptest.NewRequest(
				http.MethodGet,
				"/api/v3/dependencies?startTime="+startTime.Format(time.RFC3339Nano)+"&endTime="+endTime.Format(time.RFC3339Nano),
				nil,
			)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code, "Response body: %s", w.Body.String())
			var resp api_v3.DependenciesResponse
			require.NoError(t, jsonpb.Unmarshal(w.Body, &resp))
			assert.Equal(t, tt.expectedResp, &resp)
			depReader.AssertExpectations(t)
		})
	}
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
			name:           "missing startTime and endTime",
			url:            "/api/v3/dependencies",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "startTime",
		},
		{
			name:           "missing startTime",
			url:            "/api/v3/dependencies?endTime=" + goodEndTime,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "startTime",
		},
		{
			name:           "missing endTime",
			url:            "/api/v3/dependencies?startTime=" + goodStartTime,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "endTime",
		},
		{
			name:           "invalid startTime",
			url:            "/api/v3/dependencies?startTime=invalid&endTime=" + goodEndTime,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "startTime",
		},
		{
			name:           "invalid endTime",
			url:            "/api/v3/dependencies?startTime=" + goodStartTime + "&endTime=invalid",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "endTime",
		},
		{
			name:           "endTime not after startTime",
			url:            "/api/v3/dependencies?startTime=" + goodEndTime + "&endTime=" + goodStartTime,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "endTime",
		},
		{
			name:           "equal startTime and endTime",
			url:            "/api/v3/dependencies?startTime=" + goodStartTime + "&endTime=" + goodStartTime,
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
		"/api/v3/dependencies?startTime="+startTime.Format(time.RFC3339Nano)+"&endTime="+endTime.Format(time.RFC3339Nano),
		nil,
	)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	depReader.AssertExpectations(t)
}
