// Copyright (c) 2022 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package adjuster

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/model"
)

func TestParentReference(t *testing.T) {
	selfTraceID := model.NewTraceID(0, 1)
	otherTraceID := model.NewTraceID(0, 2)

	testCases := []struct {
		incoming []model.SpanRef
		expected []model.SpanRef
	}{
		{
			incoming: []model.SpanRef{},
			expected: []model.SpanRef{},
		},
		{
			incoming: []model.SpanRef{{TraceID: selfTraceID, RefType: model.ChildOf}},
			expected: []model.SpanRef{{TraceID: selfTraceID, RefType: model.ChildOf}},
		},
		{
			incoming: []model.SpanRef{{TraceID: otherTraceID, RefType: model.ChildOf}},
			expected: []model.SpanRef{{TraceID: otherTraceID, RefType: model.ChildOf}},
		},
		{
			incoming: []model.SpanRef{{TraceID: selfTraceID, RefType: model.ChildOf}, {TraceID: otherTraceID, RefType: model.ChildOf}},
			expected: []model.SpanRef{{TraceID: selfTraceID, RefType: model.ChildOf}, {TraceID: otherTraceID, RefType: model.ChildOf}},
		},
		{
			incoming: []model.SpanRef{{TraceID: otherTraceID, RefType: model.ChildOf}, {TraceID: selfTraceID, RefType: model.ChildOf}},
			expected: []model.SpanRef{{TraceID: selfTraceID, RefType: model.ChildOf}, {TraceID: otherTraceID, RefType: model.ChildOf}},
		},
		{
			incoming: []model.SpanRef{{TraceID: otherTraceID, RefType: model.FollowsFrom}, {TraceID: selfTraceID, RefType: model.ChildOf}},
			expected: []model.SpanRef{{TraceID: selfTraceID, RefType: model.ChildOf}, {TraceID: otherTraceID, RefType: model.FollowsFrom}},
		},
		{
			incoming: []model.SpanRef{{TraceID: otherTraceID, RefType: model.ChildOf}, {TraceID: selfTraceID, RefType: model.FollowsFrom}},
			expected: []model.SpanRef{{TraceID: selfTraceID, RefType: model.FollowsFrom}, {TraceID: otherTraceID, RefType: model.ChildOf}},
		},
		{
			incoming: []model.SpanRef{{TraceID: otherTraceID, RefType: model.ChildOf}, {TraceID: selfTraceID, RefType: model.FollowsFrom}},
			expected: []model.SpanRef{{TraceID: selfTraceID, RefType: model.FollowsFrom}, {TraceID: otherTraceID, RefType: model.ChildOf}},
		},
		{
			incoming: []model.SpanRef{{TraceID: otherTraceID, RefType: model.ChildOf}, {TraceID: selfTraceID, RefType: model.FollowsFrom}, {TraceID: selfTraceID, RefType: model.ChildOf}},
			expected: []model.SpanRef{{TraceID: selfTraceID, RefType: model.ChildOf}, {TraceID: otherTraceID, RefType: model.ChildOf}, {TraceID: selfTraceID, RefType: model.FollowsFrom}},
		},
	}
	for _, testCase := range testCases {
		trace := &model.Trace{
			Spans: []*model.Span{
				{
					TraceID:    selfTraceID,
					References: testCase.incoming,
				},
			},
		}
		trace, err := ParentReference().Adjust(trace)
		assert.NoError(t, err)
		assert.Equal(t, testCase.expected, trace.Spans[0].References)
	}
}
