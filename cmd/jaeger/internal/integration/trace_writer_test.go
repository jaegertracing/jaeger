// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
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

func TestWriteTracesInChunks(t *testing.T) {
	td := ptrace.NewTraces()
	rspans := td.ResourceSpans()

	rspan1 := rspans.AppendEmpty()
	rspan1.Resource().Attributes().PutStr("service.name", "NoServiceName1")
	scope1 := rspan1.ScopeSpans()
	sspan1 := scope1.AppendEmpty()
	sspan1.Scope().SetName("success-op-1")
	span1 := sspan1.Spans().AppendEmpty()
	span1.SetName("span1")
	span1.SetSpanID(pcommon.SpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 1}))
	span2 := sspan1.Spans().AppendEmpty()
	span2.SetName("span2")
	span2.SetSpanID(pcommon.SpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 2}))

	rspan2 := rspans.AppendEmpty()
	rspan2.Resource().Attributes().PutStr("service.name", "NoServiceName2")
	scope2 := rspan2.ScopeSpans()
	sspan2 := scope2.AppendEmpty()
	sspan2.Scope().SetName("success-op-2")
	span3 := sspan2.Spans().AppendEmpty()
	span3.SetName("span3")
	span3.SetSpanID(pcommon.SpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 3}))

	rspan3 := rspans.AppendEmpty()
	rspan3.Resource().Attributes().PutStr("service.name", "NoServiceName3")
	scope3 := rspan3.ScopeSpans()
	sspan3 := scope3.AppendEmpty()
	sspan3.Scope().SetName("success-op-3")
	span4 := sspan3.Spans().AppendEmpty()
	span4.SetName("span4")
	span4.SetSpanID(pcommon.SpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 4}))

	span5 := sspan3.Spans().AppendEmpty()
	span5.SetName("span5")
	span5.SetSpanID(pcommon.SpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 5}))

	mockExporter := &MockExporter{}
	tw := &traceWriter{
		logger:   zaptest.NewLogger(t),
		exporter: mockExporter,
	}
	mockExporter.On("ConsumeTraces", mock.Anything, mock.Anything).Return(nil).Times(3)

	err := tw.writeTraceInChunks(context.Background(), td, 2)

	require.NoError(t, err)
	mockExporter.AssertNumberOfCalls(t, "ConsumeTraces", 3)
	mockExporter.AssertExpectations(t)

	chunks := mockExporter.chunks
	require.Len(t, chunks, 3)

	assert.Equal(t, 1, chunks[0].ResourceSpans().Len()) // First chunk has 2 spans from one resource
	assert.Equal(t, 2, chunks[1].ResourceSpans().Len()) // Second chunk has 2 spans from two resources
	assert.Equal(t, 1, chunks[2].ResourceSpans().Len()) // Third chunk has one span from one resource

	// First chunk: NoServiceName1 span1 and 2
	firstChunkResource1 := chunks[0].ResourceSpans().At(0)
	assert.Equal(t, "NoServiceName1", firstChunkResource1.Resource().Attributes().AsRaw()["service.name"])
	firstChunkScope1 := firstChunkResource1.ScopeSpans().At(0)
	assert.Equal(t, "span1", firstChunkScope1.Spans().At(0).Name())
	assert.Equal(t, "span2", firstChunkScope1.Spans().At(1).Name())

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
