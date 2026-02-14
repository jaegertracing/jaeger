// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func TestDedupeSpans(t *testing.T) {
	trace := ptrace.NewTraces()
	spans := trace.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans()

	span1 := spans.AppendEmpty()
	span1.SetSpanID(pcommon.SpanID([8]byte{1}))

	span2 := spans.AppendEmpty()
	span2.SetSpanID(pcommon.SpanID([8]byte{1}))

	span3 := spans.AppendEmpty()
	span3.SetSpanID(pcommon.SpanID([8]byte{2}))

	dedupeSpans(trace)
	if trace.SpanCount() != 2 {
		t.Errorf("Expected 2 spans, got %d", trace.SpanCount())
	}
}
