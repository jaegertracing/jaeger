// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap/zaptest"
)

type MockExporter struct {
	mock.Mock
	chunks []ptrace.Traces
}

func (m *MockExporter) ConsumeTraces(ctx context.Context, td ptrace.Traces) error {
	m.chunks = append(m.chunks, td)
	args := m.Called(ctx, td)
	return args.Error(0)
}

func (m *MockExporter) Capabilities() consumer.Capabilities {
	args := m.Called()
	return args.Get(0).(consumer.Capabilities)
}

func (*MockExporter) Shutdown(context.Context) error {
	return nil
}

func (*MockExporter) Start(context.Context, component.Host) error {
	return nil
}

func TestWriteTraces(t *testing.T) {
	td := ptrace.NewTraces()
	resources := td.ResourceSpans()

	// Create resources and scopes
	for i := 1; i <= 3; i++ {
		resource := resources.AppendEmpty()
		resource.Resource().Attributes().PutStr("service.name", fmt.Sprintf("NoServiceName%d", i))
		scope := resource.ScopeSpans().AppendEmpty()
		scope.Scope().SetName(fmt.Sprintf("success-op-%d", i))
	}

	// Add span1 and span2
	scope1 := resources.At(0).ScopeSpans().At(0)
	for i := 1; i <= 2; i++ {
		span := scope1.Spans().AppendEmpty()
		span.SetSpanID(pcommon.SpanID([8]byte{0, 0, 0, 0, 0, 0, 0, byte(i)}))
		span.SetName(fmt.Sprintf("span%d", i))
	}

	// span3
	scope2 := resources.At(1).ScopeSpans().At(0)
	span3 := scope2.Spans().AppendEmpty()
	span3.SetSpanID(pcommon.SpanID([8]byte{0, 0, 0, 0, 0, 0, 3}))
	span3.SetName("span3")

	//  span4 and span5
	scope3 := resources.At(2).ScopeSpans().At(0)
	for i := 1; i <= 2; i++ {
		span := scope3.Spans().AppendEmpty()
		span.SetSpanID(pcommon.SpanID([8]byte{0, 0, 0, 0, 0, 0, 0, byte(i + 3)}))
		span.SetName(fmt.Sprintf("span%d", i+3))
	}

	mockExporter := &MockExporter{}
	mockExporter.On("ConsumeTraces", mock.Anything, mock.Anything).Return(nil).Times(3)
	tw := &traceWriter{
		logger:   zaptest.NewLogger(t),
		exporter: mockExporter,
	}
	origMaxChunkSize := MaxChunkSize
	MaxChunkSize = 2
	err := tw.WriteTraces(context.Background(), td)
	MaxChunkSize = origMaxChunkSize

	require.NoError(t, err)
	mockExporter.AssertNumberOfCalls(t, "ConsumeTraces", 3)
	mockExporter.AssertExpectations(t)

	chunks := mockExporter.chunks
	require.Len(t, chunks, 3)

	assert.Equal(t, 1, chunks[0].ResourceSpans().Len()) // First chunk has 2 spans from one resource
	assert.Equal(t, 2, chunks[1].ResourceSpans().Len()) // Second chunk has 2 spans from two resources
	assert.Equal(t, 1, chunks[2].ResourceSpans().Len()) // Third chunk has one span from one resource

	// First chunk: NoServiceName1 span1 and 2
	firstChunkResource := chunks[0].ResourceSpans().At(0)
	assert.Equal(t, "NoServiceName1", firstChunkResource.Resource().Attributes().AsRaw()["service.name"])
	firstChunkScope := firstChunkResource.ScopeSpans().At(0)
	assert.Equal(t, "span1", firstChunkScope.Spans().At(0).Name())
	assert.Equal(t, "span2", firstChunkScope.Spans().At(1).Name())

	// Second chunk: NoServiceName2 span3 and 4
	secondChunkResource := chunks[1].ResourceSpans().At(0)
	assert.Equal(t, "NoServiceName2", secondChunkResource.Resource().Attributes().AsRaw()["service.name"])
	secondChunkScope := secondChunkResource.ScopeSpans().At(0)
	assert.Equal(t, "span3", secondChunkScope.Spans().At(0).Name())

	secondChunkResource2 := chunks[1].ResourceSpans().At(1)
	assert.Equal(t, "NoServiceName3", secondChunkResource2.Resource().Attributes().AsRaw()["service.name"])
	secondChunkScope2 := secondChunkResource2.ScopeSpans().At(0)
	assert.Equal(t, "span4", secondChunkScope2.Spans().At(0).Name())

	// Third chunk: NoServiceName3 span5
	thirdChunkResource := chunks[2].ResourceSpans().At(0)
	assert.Equal(t, "NoServiceName3", thirdChunkResource.Resource().Attributes().AsRaw()["service.name"])
	thirdChunkScope := thirdChunkResource.ScopeSpans().At(0)
	assert.Equal(t, "span5", thirdChunkScope.Spans().At(0).Name())
}
