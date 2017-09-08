// Copyright (c) 2017 Uber Technologies, Inc.
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

	"github.com/uber/jaeger/model"
)

func TestSortLogFields(t *testing.T) {
	testCases := []struct {
		fields   model.KeyValues
		expected model.KeyValues
	}{
		{
			fields: model.KeyValues{
				model.String("event", "some event"), // event already in the first position, and no other fields
			},
			expected: model.KeyValues{
				model.String("event", "some event"),
			},
		},
		{
			fields: model.KeyValues{
				model.Int64("event", 42), // non-string event field
				model.Int64("a", 41),
			},
			expected: model.KeyValues{
				model.Int64("a", 41),
				model.Int64("event", 42),
			},
		},
		{
			fields: model.KeyValues{
				model.String("nonsense", "42"), // no event field
			},
			expected: model.KeyValues{
				model.String("nonsense", "42"),
			},
		},
		{
			fields: model.KeyValues{
				model.String("event", "some event"),
				model.Int64("a", 41),
			},
			expected: model.KeyValues{
				model.String("event", "some event"),
				model.Int64("a", 41),
			},
		},
		{
			fields: model.KeyValues{
				model.Int64("x", 1),
				model.Int64("a", 2),
				model.String("event", "some event"),
			},
			expected: model.KeyValues{
				model.String("event", "some event"),
				model.Int64("a", 2),
				model.Int64("x", 1),
			},
		},
	}
	for _, testCase := range testCases {
		trace := &model.Trace{
			Spans: []*model.Span{
				{
					Logs: []model.Log{
						{
							Fields: testCase.fields,
						},
					},
				},
			},
		}
		trace, err := SortLogFields().Adjust(trace)
		assert.NoError(t, err)
		assert.Equal(t, testCase.expected, model.KeyValues(trace.Spans[0].Logs[0].Fields))
	}
}
