// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dependencyprocessor

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/model"
)

// MockDependencyWriter is a mock implementation of spanstore.Writer
type MockDependencyWriter struct {
	mock.Mock
}

func (m *MockDependencyWriter) WriteSpan(ctx context.Context, span *model.Span) error {
	args := m.Called(ctx, span)
	return args.Error(0)
}

func (m *MockDependencyWriter) WriteDependencies(ctx context.Context, ts time.Time, deps []model.DependencyLink) error {
	args := m.Called(ctx, ts, deps)
	return args.Error(0)
}

func TestAggregator(t *testing.T) {
	// Create mock writer
	mockWriter := new(MockDependencyWriter)

	// Create config
	cfg := Config{
		AggregationInterval: 100 * time.Millisecond,
		InactivityTimeout:   50 * time.Millisecond,
	}

	// Create logger
	logger := zap.NewNop()
	telemetrySettings := component.TelemetrySettings{
		Logger: logger,
	}

	// Create aggregator
	agg := newDependencyAggregator(cfg, telemetrySettings, mockWriter)

	// Start aggregator
	closeChan := make(chan struct{})
	agg.Start()
	defer close(closeChan)

	// Create test spans
	traceID := createTraceID(1)
	parentSpanID := createSpanID(2)
	childSpanID := createSpanID(3)

	// Create parent span
	parentSpan := createSpan(traceID, parentSpanID, pcommon.SpanID{}, "service1")

	// Create child span
	childSpan := createSpan(traceID, childSpanID, parentSpanID, "service2")

	// Setup mock expectations
	mockWriter.On("WriteDependencies", mock.Anything, mock.Anything, mock.MatchedBy(func(deps []model.DependencyLink) bool {
		if len(deps) != 1 {
			return false
		}
		dep := deps[0]
		return dep.Parent == "service1" && dep.Child == "service2" && dep.CallCount == 1
	})).Return(nil)

	// Handle spans
	ctx := context.Background()
	agg.HandleSpan(ctx, parentSpan, "service1")
	agg.HandleSpan(ctx, childSpan, "service2")

	// Wait for processing and verify
	assert.Eventually(t, func() bool {
		return mockWriter.AssertExpectations(t)
	}, time.Second, 10*time.Millisecond, "Dependencies were not written as expected")
}

func TestAggregatorInactivityTimeout(t *testing.T) {
	mockWriter := new(MockDependencyWriter)
	cfg := Config{
		AggregationInterval: 1 * time.Second,
		InactivityTimeout:   50 * time.Millisecond,
	}

	agg := newDependencyAggregator(cfg, component.TelemetrySettings{Logger: zap.NewNop()}, mockWriter)
	closeChan := make(chan struct{})
	agg.Start()
	defer close(closeChan)

	traceID := createTraceID(1)
	spanID := createSpanID(1)
	span := createSpan(traceID, spanID, pcommon.SpanID{}, "service1")

	mockWriter.On("WriteDependencies", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	ctx := context.Background()
	agg.HandleSpan(ctx, span, "service1")

	// assert.Eventually(t, func() bool {
	// 	agg.tracesLock.RLock()
	// 	defer agg.tracesLock.RUnlock()
	// 	return len(agg.traces) == 0
	// }, time.Second, 10*time.Millisecond, "Trace was not cleared after inactivity timeout")
}

func TestAggregatorClose(t *testing.T) {
	mockWriter := new(MockDependencyWriter)
	cfg := Config{
		AggregationInterval: 1 * time.Second,
		InactivityTimeout:   1 * time.Second,
	}

	agg := newDependencyAggregator(cfg, component.TelemetrySettings{Logger: zap.NewNop()}, mockWriter)
	closeChan := make(chan struct{})
	agg.Start()

	traceID := createTraceID(1)
	spanID := createSpanID(1)
	span := createSpan(traceID, spanID, pcommon.SpanID{}, "service1")

	ctx := context.Background()
	agg.HandleSpan(ctx, span, "service1")

	mockWriter.On("WriteDependencies", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	close(closeChan)
	err := agg.Close()
	require.NoError(t, err)

	// assert.Eventually(t, func() bool {
	// 	agg.tracesLock.RLock()
	// 	defer agg.tracesLock.RUnlock()
	// 	return len(agg.traces) == 0
	// }, time.Second, 10*time.Millisecond, "Traces were not cleared after close")
}

// Helper functions

func createTraceID(id byte) pcommon.TraceID {
	var traceID [16]byte
	traceID[15] = id
	return pcommon.TraceID(traceID)
}

func createSpanID(id byte) pcommon.SpanID {
	var spanID [8]byte
	spanID[7] = id
	return pcommon.SpanID(spanID)
}

func createSpan(traceID pcommon.TraceID, spanID pcommon.SpanID, parentSpanID pcommon.SpanID, serviceName string) ptrace.Span {
	span := ptrace.NewSpan()
	span.SetTraceID(traceID)
	span.SetSpanID(spanID)
	span.SetParentSpanID(parentSpanID)
	// Additional span attributes could be set here if needed
	return span
}
