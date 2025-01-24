// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger-idl/model/v1"
)

func TestDedupeSpans(t *testing.T) {
	trace := &model.Trace{
		Spans: []*model.Span{
			{
				SpanID: 1,
			},
			{
				SpanID: 1,
			},
			{
				SpanID: 2,
			},
		},
	}
	dedupeSpans(trace)
	assert.Len(t, trace.Spans, 2)
}
