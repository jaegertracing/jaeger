// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/storage/integration"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/v1adapter"
)

func TestJaegerQueryService(t *testing.T) {
	integration.SkipUnlessEnv(t, "query")
	s := &E2EStorageIntegration{
		ConfigFile: "../../config-query.yaml",
		StorageIntegration: integration.StorageIntegration{
			CleanUp: func(_ *testing.T) {}, // nothing to clean
		},
		HealthCheckPort:    12133, // referencing value in config-query.yaml
		MetricsPort:        8887,
		SkipStorageCleaner: true,
	}
	s.e2eInitialize(t, "memory")

	runTraceReaderSmokeTests(context.Background(), s.TraceReader, t)
}

// RunTraceReaderSmokeTests runs tests on TraceReader to see it returns no error
func runTraceReaderSmokeTests(ctx context.Context, traceReader tracestore.Reader, t *testing.T) {
	t.Run("TraceReader.GetTraces", func(t *testing.T) {
		iterTraces := traceReader.GetTraces(ctx)
		_, err := v1adapter.V1TracesFromSeq2(iterTraces)
		assert.NoError(t, err)
	})

	t.Run("TraceReader.GetServices", func(t *testing.T) {
		_, err := traceReader.GetServices(ctx)
		assert.NoError(t, err)
	})

	t.Run("TraceReader.GetOperations", func(t *testing.T) {
		_, err := traceReader.GetOperations(ctx, tracestore.OperationQueryParams{ServiceName: "random-service-name"})
		assert.NoError(t, err)
	})

	t.Run("TraceReader.FindTraces", func(t *testing.T) {
		iter := traceReader.FindTraces(ctx, tracestore.TraceQueryParams{
			ServiceName:  "random-service-name",
			Attributes:   pcommon.NewMap(),
			StartTimeMin: time.Now().Add(-2 * time.Hour),
			StartTimeMax: time.Now(),
		})
		iter(func(_ []ptrace.Traces, err error) bool {
			return assert.NoError(t, err)
		})
	})
}
