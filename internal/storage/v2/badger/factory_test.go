// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package badger

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/jaegertracing/jaeger/internal/storage/v1/badger"
	"github.com/jaegertracing/jaeger/internal/telemetry"
)

func TestNewFac(t *testing.T) {
	telset := telemetry.NoopSettings()
	telset.Logger = zaptest.NewLogger(t, zaptest.WrapOptions(zap.AddCaller()))
	f, err := NewFactory(*badger.DefaultConfig(), telset)
	require.NoError(t, err)

	_, err = f.CreateTraceReader()
	require.NoError(t, err)

	_, err = f.CreateTraceWriter()
	require.NoError(t, err)

	_, err = f.CreateDependencyReader()
	require.NoError(t, err)

	_, err = f.CreateSamplingStore(5)
	require.NoError(t, err)

	lock, err := f.CreateLock()
	require.NoError(t, err)
	assert.NotNil(t, lock)

	err = f.Purge(context.Background())
	require.NoError(t, err)

	err = f.Close()
	require.NoError(t, err)
}

func TestBadgerStorageFactoryWithConfig(t *testing.T) {
	t.Parallel()
	cfg := badger.Config{}
	_, err := NewFactory(cfg, telemetry.NoopSettings())
	require.ErrorContains(t, err, "Error Creating Dir: \"\" err: mkdir : no such file or directory")

	cfg = badger.Config{
		Ephemeral:             true,
		MaintenanceInterval:   5,
		MetricsUpdateInterval: 10,
	}
	factory, err := NewFactory(cfg, telemetry.NoopSettings())
	require.NoError(t, err)
	factory.Close()
}

func TestGetServices(t *testing.T) {
	cfg := *badger.DefaultConfig()
	cfg.Ephemeral = true
	cfg.MaintenanceInterval = 5
	cfg.MetricsUpdateInterval = 10
	telset := telemetry.NoopSettings()
	telset.Logger = zaptest.NewLogger(t, zaptest.WrapOptions(zap.AddCaller()))
	factory, err := NewFactory(cfg, telset)
	require.NoError(t, err)
	defer factory.Close()

	writer, err := factory.CreateTraceWriter()
	require.NoError(t, err)

	reader, err := factory.CreateTraceReader()
	require.NoError(t, err)

	// Write traces for multiple services to test service discovery
	expectedServices := []string{"service-a", "service-b", "service-c"}
	for i, service := range expectedServices {
		traces := createTestTraces(service, "operation-1", byte(i+1))
		err = writer.WriteTraces(context.Background(), traces)
		require.NoError(t, err)
	}

	// Retrieve and verify all services are discovered
	actualServices, err := reader.GetServices(context.Background())
	require.NoError(t, err)
	require.Len(t, actualServices, len(expectedServices))
	assert.Equal(t, expectedServices, actualServices)
}

func createTestTraces(serviceName, operationName string, traceIDSuffix byte) ptrace.Traces {
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", serviceName)

	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()
	span.SetName(operationName)
	now := time.Now()
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(now))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(now.Add(time.Second)))
	span.SetTraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, traceIDSuffix})
	span.SetSpanID([8]byte{0, 0, 0, 0, 0, 0, 0, traceIDSuffix})

	return traces
}
