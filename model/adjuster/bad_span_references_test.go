// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"testing"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/stretchr/testify/assert"
)

func TestSpanReferencesAdjuster(t *testing.T) {
	trace := &model.Trace{
		Spans: []*model.Span{
			{},
			{
				References: []model.SpanRef{},
			},
			{
				References: []model.SpanRef{
					{TraceID: model.NewTraceID(0, 1)},
					{TraceID: model.NewTraceID(1, 0)},
					{TraceID: model.NewTraceID(0, 0)},
				},
			},
		},
	}
	SpanReferences().Adjust(trace)
	assert.Empty(t, trace.Spans[0].References)
	assert.Empty(t, trace.Spans[1].References)
	assert.Len(t, trace.Spans[2].References, 2)
	assert.Contains(t, trace.Spans[2].Warnings[0], "Invalid span reference removed")
}
