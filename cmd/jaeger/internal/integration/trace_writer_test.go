// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"

	"github.com/crossdock/crossdock-go/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func TestChunkTraces_GetChunkedTracesWithValidInput(t *testing.T) {
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

	// Set chunk size to 2
	chunks := ChunkTraces(td, 2)

	require.Len(t, chunks, 3)

	require.Equal(t, chunks[0].ResourceSpans().Len(), 1) // First chunk has 2 spans from one resource
	require.Equal(t, chunks[1].ResourceSpans().Len(), 2) // Second chunk has 2 spans from two resources
	require.Equal(t, chunks[2].ResourceSpans().Len(), 1) // Third chunk has one span from one resource

	// First chunk: NoServiceName1 span1 and 2
	firstChunkResource1 := chunks[0].ResourceSpans().At(0)
	require.Equal(t, "NoServiceName1", firstChunkResource1.Resource().Attributes().AsRaw()["service.name"])
	firstChunkScope1 := firstChunkResource1.ScopeSpans().At(0)
	require.Equal(t, "span1", firstChunkScope1.Spans().At(0).Name())
	require.Equal(t, "span2", firstChunkScope1.Spans().At(1).Name())

	// Second chunk: NoServiceName2 span3 and 4
	secondChunkResource := chunks[1].ResourceSpans().At(0)
	require.Equal(t, "NoServiceName2", secondChunkResource.Resource().Attributes().AsRaw()["service.name"])
	secondChunkScope := secondChunkResource.ScopeSpans().At(0)
	require.Equal(t, "span3", secondChunkScope.Spans().At(0).Name())

	secondChunkResource2 := chunks[1].ResourceSpans().At(1)
	require.Equal(t, "NoServiceName3", secondChunkResource2.Resource().Attributes().AsRaw()["service.name"])
	secondChunkScope2 := secondChunkResource2.ScopeSpans().At(0)
	require.Equal(t, "span4", secondChunkScope2.Spans().At(0).Name())

	// Third chunk: NoServiceName3 span5
	thirdChunkResource := chunks[2].ResourceSpans().At(0)
	require.Equal(t, "NoServiceName3", thirdChunkResource.Resource().Attributes().AsRaw()["service.name"])
	thirdChunkScope := thirdChunkResource.ScopeSpans().At(0)
	require.Equal(t, "span5", thirdChunkScope.Spans().At(0).Name())
}

func TestChunkTraces_InvalidInput(t *testing.T) {
	td := ptrace.NewTraces()

	chunks := ChunkTraces(td, 2)
	require.Len(t, chunks, 0)
}
