// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func TestIPAttributeAdjuster(t *testing.T) {
	traces := ptrace.NewTraces()
	resourceSpans := traces.ResourceSpans().AppendEmpty()

	resourceSpans.Resource().Attributes().PutInt("a", 42)
	resourceSpans.Resource().Attributes().PutInt("ip", 1<<26|2<<16|3<<8|4)
	resourceSpans.Resource().Attributes().PutStr("peer.ipv4", "something")

	spans := resourceSpans.ScopeSpans().AppendEmpty().Spans()

	spanA := spans.AppendEmpty()
	spanA.Attributes().PutInt("a", 42)
	spanA.Attributes().PutStr("ip", "not integer")
	spanA.Attributes().PutInt("ip", 1<<24|2<<16|3<<8|4)
	spanA.Attributes().PutStr("peer.ipv4", "something else")

	spanB := spans.AppendEmpty()
	spanB.Attributes().PutDouble("ip", 1<<25|2<<16|3<<8|4)

	trace, err := IPAttribute().Adjust(traces)
	require.NoError(t, err)

	resourceSpan := trace.ResourceSpans().At(0)
	resourceAttributes := resourceSpan.Resource().Attributes()

	val, ok := resourceAttributes.Get("a")
	require.True(t, ok)
	require.EqualValues(t, 42, val.Int())

	val, ok = resourceAttributes.Get("ip")
	require.True(t, ok)
	require.EqualValues(t, "4.2.3.4", val.Str())

	val, ok = resourceAttributes.Get("peer.ipv4")
	require.True(t, ok)
	require.EqualValues(t, "something", val.Str())

	span := resourceSpan.ScopeSpans().At(0).Spans()
	attributesA := span.At(0).Attributes()

	val, ok = attributesA.Get("a")
	require.True(t, ok)
	require.EqualValues(t, 42, val.Int())

	val, ok = attributesA.Get("ip")
	require.True(t, ok)
	require.EqualValues(t, "1.2.3.4", val.Str())

	val, ok = attributesA.Get("peer.ipv4")
	require.True(t, ok)
	require.EqualValues(t, "something else", val.Str())

	attributesB := span.At(1).Attributes()

	val, ok = attributesB.Get("ip")
	require.True(t, ok)
	require.EqualValues(t, "2.2.3.4", val.Str())
}
