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

func TestIPTagAdjuster(t *testing.T) {
	trace := &model.Trace{
		Spans: []*model.Span{
			{
				Tags: model.KeyValues{
					model.Int64("a", 42),
					model.String("ip", "not integer"),
					model.Int64("ip", 1<<24|2<<16|3<<8|4),
				},
				Process: &model.Process{
					Tags: model.KeyValues{
						model.Int64("a", 42),
						model.String("ip", "not integer"),
						model.Int64("ip", 1<<24|2<<16|3<<8|4),
					},
				},
			},
		},
	}
	trace, err := IPTagAdjuster().Adjust(trace)
	assert.NoError(t, err)

	expectedSpanTags := model.KeyValues{
		model.Int64("a", 42),
		model.String("ip", "not integer"),
		model.String("ip", "1.2.3.4"),
	}
	assert.Equal(t, expectedSpanTags, trace.Spans[0].Tags)

	expectedProcessTags := model.KeyValues{
		model.Int64("a", 42),
		model.String("ip", "1.2.3.4"), // sorted
		model.String("ip", "not integer"),
	}
	assert.Equal(t, expectedProcessTags, trace.Spans[0].Process.Tags)
}
