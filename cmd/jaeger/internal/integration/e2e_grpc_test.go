// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/jiter"
	"github.com/jaegertracing/jaeger/internal/storage/integration"
	"github.com/jaegertracing/jaeger/internal/storage/integration/capabilities"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/ports"
)

func TestGRPCStorage(t *testing.T) {
	integration.SkipUnlessEnv(t, integration.StorageGRPC)

	if os.Getenv("SKIP_STARTING_BACKEND") != "true" {
		remoteBackend := &E2EStorageIntegration{
			ConfigFile:      "../../config-remote-storage-backend.yaml",
			HealthCheckPort: 12133,
			MetricsPort:     8887,
			EnvVarOverrides: map[string]string{
				"REMOTE_STORAGE_BACKEND_GRPC_ENDPOINT": "0.0.0.0:4316",
			},
		}
		remoteBackend.e2eInitialize(t, "memory")
		t.Log("Remote backend initialized")
	} else {
		t.Log("Skipping remote backend initialization; SKIP_STARTING_BACKEND is enabled")
	}

	collector := &E2EStorageIntegration{
		FeatureGates:       append([]string{querysvc.StructuredFiltersGate.ID()}, paginationGates...),
		ConfigFile:         "../../config-remote-storage.yaml",
		SkipStorageCleaner: true,
		StorageIntegration: integration.StorageIntegration{
			CleanUp:      purge,
			Capabilities: capabilities.Capabilities{}.WithoutSpanAttributeOrdering(),
		},
		PropagateEnvVars: []string{
			"REMOTE_STORAGE_ENDPOINT",
			"REMOTE_STORAGE_WRITER_ENDPOINT",
		},
	}
	collector.e2eInitialize(t, "grpc")
	t.Log("Collector initialized")

	collector.RunSpanStoreTests(t)
	t.Run("PaginationErrorStatus", func(t *testing.T) { testPaginationErrorStatus(t, collector) })
}

// testPaginationErrorStatus checks that errors crossing remote storage retain their identity
// and become gRPC InvalidArgument and HTTP 400; direct storage tests do not run the query server or HTTP gateway.
func testPaginationErrorStatus(t *testing.T, collector *E2EStorageIntegration) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	query := tracestore.TraceQueryParams{
		ServiceName: "pagination-service", Attributes: pcommon.NewMap(),
		StartTimeMin: time.Now().Add(-time.Hour), StartTimeMax: time.Now(),
		Pagination: &tracestore.Pagination{PageSize: 2, PageToken: "not-a-token!"},
	}
	_, err := jiter.CollectWithErrors(collector.TraceReader.FindTraceSummaries(ctx, query))
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	params := url.Values{
		"query.serviceName":          {query.ServiceName},
		"query.startTimeMin":         {query.StartTimeMin.Format(time.RFC3339Nano)},
		"query.startTimeMax":         {query.StartTimeMax.Format(time.RFC3339Nano)},
		"query.pagination.pageSize":  {"2"},
		"query.pagination.pageToken": {string(query.Pagination.PageToken)},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+ports.PortToHostPort(ports.QueryHTTP)+"/api/v3/trace-summaries?"+params.Encode(), http.NoBody)
	require.NoError(t, err)
	resp, err := testingHttpClient(t).Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "%s", body)
	assert.Contains(t, string(body), tracestore.ErrPaginationInvalid.Error())
}
