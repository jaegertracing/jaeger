package apiv3

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	depsmocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore/mocks"
	tracemocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore/mocks"
)

func TestHTTPGatewayGetDependencies(t *testing.T) {
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

	endTime := time.Now().UTC()

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

	depReader.On("GetDependencies", mock.Anything, mock.Anything, mock.Anything).Return(expectedDeps, nil)

	router := mux.NewRouter()
	gateway.RegisterRoutes(router)

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v3/dependencies?end_time="+endTime.Format(time.RFC3339Nano)+"&lookback=24h",
		nil,
	)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "Response body: %s", w.Body.String())

	var response struct {
		Dependencies []struct {
			Parent    string `json:"parent"`
			Child     string `json:"child"`
			CallCount uint64 `json:"callCount"`
			Source    string `json:"source"`
		} `json:"dependencies"`
	}

	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	assert.Len(t, response.Dependencies, 2)
	assert.Equal(t, "frontend", response.Dependencies[0].Parent)
	assert.Equal(t, "backend", response.Dependencies[0].Child)
	assert.Equal(t, uint64(100), response.Dependencies[0].CallCount)
	assert.Equal(t, "traces", response.Dependencies[0].Source)
}

func TestHTTPGatewayGetDependenciesErrors(t *testing.T) {
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

	router := mux.NewRouter()
	gateway.RegisterRoutes(router)

	tests := []struct {
		name           string
		url            string
		expectedStatus int
	}{
		{
			name:           "missing end_time",
			url:            "/api/v3/dependencies?lookback=24h",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "missing lookback",
			url:            "/api/v3/dependencies?end_time=2026-01-24T16:00:00Z",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "invalid end_time",
			url:            "/api/v3/dependencies?end_time=invalid&lookback=24h",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "invalid lookback",
			url:            "/api/v3/dependencies?end_time=2026-01-24T16:00:00Z&lookback=invalid",
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestHTTPGatewayGetDependencies_StorageError(t *testing.T) {
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

	depReader.On("GetDependencies", mock.Anything, mock.Anything, mock.Anything).Return(nil, assert.AnError)

	router := mux.NewRouter()
	gateway.RegisterRoutes(router)

	endTime := time.Now().UTC()
	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v3/dependencies?end_time="+endTime.Format(time.RFC3339Nano)+"&lookback=24h",
		nil,
	)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
