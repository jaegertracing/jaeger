// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dependencyprocessor

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/processor/processortest"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/plugin/storage/memory"
)

func TestDependencyProcessorEndToEnd(t *testing.T) {
	// Create a mock receiver, processor, and exporter
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig().(*Config)

	// Create a mock next consumer (exporter)
	sink := new(consumertest.TracesSink)

	// Create a memory store to store dependency links
	store := memory.NewStore()

	// Create the processor
	processor, err := factory.CreateTraces(
		context.Background(),
		processortest.NewNopSettings(),
		cfg,
		sink,
	)
	require.NoError(t, err)

	// Start the processor
	err = processor.Start(context.Background(), componenttest.NewNopHost())
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, processor.Shutdown(context.Background()))
	}()

	// Create a test trace
	trace := createTestTrace()

	// Send the trace to the processor
	err = processor.ConsumeTraces(context.Background(), trace)
	require.NoError(t, err)

	// Wait for the processor to process the trace
	time.Sleep(cfg.AggregationInterval + 100*time.Millisecond)

	// Verify dependency links
	deps, err := store.GetDependencies(context.Background(), time.Now(), cfg.AggregationInterval)
	require.NoError(t, err)

	// Expected dependency links
	expectedDeps := []model.DependencyLink{
		{
			Parent:    "service1",
			Child:     "service2",
			CallCount: 1,
		},
	}
	assert.Equal(t, expectedDeps, deps, "dependency links do not match expected output")
}

// createTestTrace creates a test trace with two spans from different services.
func createTestTrace() ptrace.Traces {
	traces := ptrace.NewTraces()

	// Create a resource span for the parent span (service1)
	rs1 := traces.ResourceSpans().AppendEmpty()
	rs1.Resource().Attributes().PutStr("service.name", "service1")
	ils1 := rs1.ScopeSpans().AppendEmpty()
	parentSpan := ils1.Spans().AppendEmpty()
	parentSpan.SetTraceID([16]byte{1, 2, 3, 4})
	parentSpan.SetSpanID([8]byte{5, 6, 7, 8})
	parentSpan.SetName("span2")
	parentSpan.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Now()))
	parentSpan.SetEndTimestamp(pcommon.NewTimestampFromTime(time.Now().Add(time.Second)))

	// Create a resource span for the child span (service2)
	rs2 := traces.ResourceSpans().AppendEmpty()
	rs2.Resource().Attributes().PutStr("service.name", "service2")
	ils2 := rs2.ScopeSpans().AppendEmpty()
	span := ils2.Spans().AppendEmpty()
	span.SetTraceID([16]byte{1, 2, 3, 4})
	span.SetSpanID([8]byte{1, 2, 3, 4})
	span.SetName("span1")
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Now()))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(time.Now().Add(time.Second)))
	span.SetParentSpanID(parentSpan.SpanID()) // Set parent-child relationship

	return traces
}
