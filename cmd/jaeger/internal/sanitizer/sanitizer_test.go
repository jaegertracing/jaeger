// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package sanitizer

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func TestNewStandardSanitizers(t *testing.T) {
	sanitizers := NewStandardSanitizers()
	require.Len(t, sanitizers, 2)
}

func TestNewChainedSanitizer(t *testing.T) {
	var s1 Func = func(traces ptrace.Traces) ptrace.Traces {
		traces.
			ResourceSpans().
			AppendEmpty().
			Resource().
			Attributes().
			PutStr("hello", "world")
		return traces
	}
	var s2 Func = func(traces ptrace.Traces) ptrace.Traces {
		traces.
			ResourceSpans().
			At(0).
			Resource().
			Attributes().
			PutStr("hello", "goodbye")
		return traces
	}
	c1 := NewChainedSanitizer(s1)
	t1 := c1(ptrace.NewTraces())
	hello, ok := t1.
		ResourceSpans().
		At(0).
		Resource().
		Attributes().
		Get("hello")
	require.True(t, ok)
	require.Equal(t, "world", hello.Str())
	c2 := NewChainedSanitizer(s1, s2)
	t2 := c2(ptrace.NewTraces())
	hello, ok = t2.
		ResourceSpans().
		At(0).
		Resource().
		Attributes().
		Get("hello")
	require.True(t, ok)
	require.Equal(t, "goodbye", hello.Str())
}
