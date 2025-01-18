// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package sanitizer

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/jptrace"
)

func createTestTraces() ptrace.Traces {
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()
	span.SetName("test-span")
	return traces
}

func TestHashingSanitizer(t *testing.T) {
	t.Run("new span without hash", func(t *testing.T) {
		traces := createTestTraces()
		sanitized := hashingSanitizer(traces)

		span := sanitized.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
		hashAttr, exists := span.Attributes().Get(jptrace.HashAttribute)

		require.True(t, exists, "hash should be added")
		fmt.Printf("%v",hashAttr.Type())
		assert.Equal(t, pcommon.ValueTypeInt, hashAttr.Type())
	})
}

func TestComputeHashCode(t *testing.T) {
	t.Run("successful hash computation", func(t *testing.T) {
		traces := createTestTraces()
		marshaler := &ptrace.ProtoMarshaler{}

		hash, span, err := computeHashCode(traces, marshaler)

		require.NoError(t, err)
		assert.NotZero(t, hash)

		hashAttr, exists := span.Attributes().Get(jptrace.HashAttribute)
		require.True(t, exists)
		assert.Equal(t, int64(hash), hashAttr.Int())
	})
}
