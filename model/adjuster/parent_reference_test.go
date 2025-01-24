// Copyright (c) 2022 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger-idl/model/v1"
)

func TestParentReference(t *testing.T) {
	a := model.NewTraceID(0, 1)
	b := model.NewTraceID(0, 2)
	childOf := func(id model.TraceID) model.SpanRef {
		return model.NewChildOfRef(id, 1)
	}
	followsFrom := func(id model.TraceID) model.SpanRef {
		return model.NewFollowsFromRef(id, 1)
	}
	followsFrom2 := func(id model.TraceID) model.SpanRef {
		return model.NewFollowsFromRef(id, 2)
	}

	testCases := []struct {
		name     string
		incoming []model.SpanRef
		expected []model.SpanRef
	}{
		{
			name:     "empty",
			incoming: []model.SpanRef{},
			expected: []model.SpanRef{},
		},
		{
			name:     "single child",
			incoming: []model.SpanRef{childOf(a)},
			expected: []model.SpanRef{childOf(a)},
		},
		{
			name:     "single remote child",
			incoming: []model.SpanRef{childOf(b)},
			expected: []model.SpanRef{childOf(b)},
		},
		{
			name:     "local, local follows - keep order",
			incoming: []model.SpanRef{childOf(a), followsFrom(a)},
			expected: []model.SpanRef{childOf(a), followsFrom(a)},
		},
		{
			name:     "local and remote child in order",
			incoming: []model.SpanRef{childOf(a), childOf(b)},
			expected: []model.SpanRef{childOf(a), childOf(b)},
		},
		{
			name:     "local and remote child out of order",
			incoming: []model.SpanRef{childOf(b), childOf(a)},
			expected: []model.SpanRef{childOf(a), childOf(b)},
		},
		{
			name:     "local child, remote follows",
			incoming: []model.SpanRef{followsFrom(b), childOf(a)},
			expected: []model.SpanRef{childOf(a), followsFrom(b)},
		},
		{
			name:     "remote, local, local follows - keep order",
			incoming: []model.SpanRef{followsFrom(b), followsFrom2(a), followsFrom(a)},
			expected: []model.SpanRef{followsFrom2(a), followsFrom(b), followsFrom(a)},
		},
		{
			name:     "remote child, local follows",
			incoming: []model.SpanRef{childOf(b), followsFrom(a)},
			expected: []model.SpanRef{followsFrom(a), childOf(b)},
		},
		{
			name:     "remote child, local follows, local child",
			incoming: []model.SpanRef{childOf(b), followsFrom(a), childOf(a)},
			expected: []model.SpanRef{childOf(a), followsFrom(a), childOf(b)},
		},
		{
			name:     "remote follows, local follows, local child",
			incoming: []model.SpanRef{followsFrom(b), followsFrom(a), childOf(a)},
			expected: []model.SpanRef{childOf(a), followsFrom(a), followsFrom(b)},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			trace := &model.Trace{
				Spans: []*model.Span{
					{
						TraceID:    a,
						References: testCase.incoming,
					},
				},
			}
			ParentReference().Adjust(trace)
			assert.Equal(t, testCase.expected, trace.Spans[0].References)
		})
	}
}
