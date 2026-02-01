// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0
package memory

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

func TestResourcePrefixedAttributeFiltering(t *testing.T) {
	tenant := newTenant(&Configuration{MaxTraces: 10})

	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()

	// resource attribute
	rs.Resource().Attributes().PutStr("host", "server-1")

	ss := rs.ScopeSpans().AppendEmpty()
	ss.Scope().SetName("test-scope")

	span := ss.Spans().AppendEmpty()
	span.SetName("op-1")
	span.Attributes().PutStr("host", "span-host")

	start := time.Now()
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(start))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(start.Add(time.Second)))

	traceID := pcommon.TraceID([16]byte{1})
	span.SetTraceID(traceID)

	tenant.storeTraces(map[pcommon.TraceID]ptrace.ResourceSpansSlice{
		traceID: traces.ResourceSpans(),
	})

	// case 1: no prefix (should match resource)
	q1 := tracestore.TraceQueryParams{SearchDepth: 10}
	q1.Attributes = pcommon.NewMap()
	q1.Attributes.PutStr("host", "server-1")

	res, err := tenant.findTraceAndIds(q1)
	require.NoError(t, err)
	require.Len(t, res, 1)

	// case 2: resource prefix (correct)
	q2 := tracestore.TraceQueryParams{SearchDepth: 10}
	q2.Attributes = pcommon.NewMap()
	q2.Attributes.PutStr("resource.host", "server-1")

	res, err = tenant.findTraceAndIds(q2)
	require.NoError(t, err)
	require.Len(t, res, 1)

	// case 3: resource prefix but only on span
	q3 := tracestore.TraceQueryParams{SearchDepth: 10}
	q3.Attributes = pcommon.NewMap()
	q3.Attributes.PutStr("resource.host", "span-host")

	res, err = tenant.findTraceAndIds(q3)
	require.NoError(t, err)
	require.Empty(t, res)
}
