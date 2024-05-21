// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"

	"github.com/jaegertracing/jaeger/model"
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

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
