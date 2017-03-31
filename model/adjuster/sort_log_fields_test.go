// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

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
