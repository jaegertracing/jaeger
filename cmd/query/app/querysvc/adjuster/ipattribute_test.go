// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func TestIPAttributeAdjuster(t *testing.T) {
	traces := ptrace.NewTraces()
	resourceSpans := traces.ResourceSpans().AppendEmpty()

	resourceAttributes := resourceSpans.Resource().Attributes()
	resourceAttributes.PutInt("a", 42)
	resourceAttributes.PutInt("ip", 1<<26|2<<16|3<<8|4)
	resourceAttributes.PutStr("peer.ipv4", "something")

	spans := resourceSpans.ScopeSpans().AppendEmpty().Spans()

	createSpan := func(attrs map[string]any) {
		span := spans.AppendEmpty()
		for key, value := range attrs {
			switch v := value.(type) {
			case int:
				span.Attributes().PutInt(key, int64(v))
			case string:
				span.Attributes().PutStr(key, v)
			case float64:
				span.Attributes().PutDouble(key, v)
			}
		}
	}

	createSpan(map[string]any{
		"a":         42,
		"ip":        int(1<<24 | 2<<16 | 3<<8 | 4),
		"peer.ipv4": "something else",
	})

	createSpan(map[string]any{
		"ip": float64(1<<25 | 2<<16 | 3<<8 | 4),
	})

	assertAttribute := func(attributes pcommon.Map, key string, expected any) {
		val, ok := attributes.Get(key)
		require.True(t, ok)
		switch v := expected.(type) {
		case int:
			require.EqualValues(t, v, val.Int())
		case string:
			require.EqualValues(t, v, val.Str())
		}
	}

	trace, err := IPAttribute().Adjust(traces)
	require.NoError(t, err)

	resourceSpan := trace.ResourceSpans().At(0)
	assert.Equal(t, 3, resourceSpan.Resource().Attributes().Len())

	assertAttribute(resourceSpan.Resource().Attributes(), "a", 42)
	assertAttribute(resourceSpan.Resource().Attributes(), "ip", "4.2.3.4")
	assertAttribute(resourceSpan.Resource().Attributes(), "peer.ipv4", "something")

	gotSpans := resourceSpan.ScopeSpans().At(0).Spans()
	assert.Equal(t, 2, gotSpans.Len())

	assert.Equal(t, 3, gotSpans.At(0).Attributes().Len())
	assertAttribute(gotSpans.At(0).Attributes(), "a", 42)
	assertAttribute(gotSpans.At(0).Attributes(), "ip", "1.2.3.4")
	assertAttribute(gotSpans.At(0).Attributes(), "peer.ipv4", "something else")

	assert.Equal(t, 1, gotSpans.At(1).Attributes().Len())
	assertAttribute(gotSpans.At(1).Attributes(), "ip", "2.2.3.4")
}
