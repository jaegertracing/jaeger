// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"fmt"
	"testing"

	"github.com/jaegertracing/jaeger-idl/model/v1"
)

func verifyDedupeSpans(trace *model.Trace) error {
	dedupeSpans(trace)

	if len(trace.Spans) != 2 {
		return fmt.Errorf("expected 2 spans after dedupe, got %d", len(trace.Spans))
	}
	return nil
}

func TestDedupeSpans(t *testing.T) {
	trace := &model.Trace{
		Spans: []*model.Span{
			{SpanID: 1},
			{SpanID: 1},
			{SpanID: 2},
		},
	}

	if err := verifyDedupeSpans(trace); err != nil {
		t.Fatal(err)
	}
}
