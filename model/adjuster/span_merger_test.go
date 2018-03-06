// Copyright (c) 2018 The Jaeger Authors.
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
	"time"

	"github.com/jaegertracing/jaeger/model"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/stretchr/testify/assert"
)

func Test_MergeAdjuster(t *testing.T) {
	tests := []struct {
		name     string
		input    *model.Trace
		expected *model.Trace
	}{
		{
			name: "non duplicated spans: do nothing",
			input: &model.Trace{
				Spans: []*model.Span{
					{
						SpanID: model.SpanID(1),
					},
					{
						SpanID: model.SpanID(2),
					},
					{
						SpanID: model.SpanID(3),
					},
				},
			},
			expected: &model.Trace{
				Spans: []*model.Span{
					{
						SpanID: model.SpanID(1),
					},
					{
						SpanID: model.SpanID(2),
					},
					{
						SpanID: model.SpanID(3),
					},
				},
			},
		},
		{
			name: "duplicate Jaeger spans: select longest duration",
			input: &model.Trace{
				Spans: []*model.Span{
					{
						SpanID: model.SpanID(1),
					},
					{
						SpanID:   model.SpanID(1),
						Duration: 1 * time.Microsecond,
					},
					{
						SpanID:   model.SpanID(1),
						Duration: 5 * time.Microsecond,
					},
					{
						SpanID:   model.SpanID(1),
						Duration: 5 * time.Microsecond,
					},
					{
						SpanID:   model.SpanID(1),
						Duration: 2 * time.Microsecond,
					},
					{
						SpanID: model.SpanID(2),
					},
				},
			},
			expected: &model.Trace{
				Spans: []*model.Span{
					{
						SpanID:   model.SpanID(1),
						Duration: 5 * time.Microsecond,
					},
					{
						SpanID: model.SpanID(2),
					},
				},
			},
		},
		{
			name: "duplicate Zipkin spans: don't merge",
			input: &model.Trace{
				Spans: []*model.Span{
					{
						SpanID:   model.SpanID(1),
						Duration: 1 * time.Microsecond,
						Tags: model.KeyValues{
							model.String(string(ext.SpanKind), string(ext.SpanKindRPCClientEnum)),
						},
					},
					{
						SpanID:   model.SpanID(1),
						Duration: 5 * time.Microsecond,
						Tags: model.KeyValues{
							model.String(string(ext.SpanKind), string(ext.SpanKindRPCClientEnum)),
						},
					},
					{
						SpanID:   model.SpanID(1),
						Duration: 2 * time.Microsecond,
						Tags: model.KeyValues{
							model.String(string(ext.SpanKind), string(ext.SpanKindRPCServerEnum)),
						},
					},
					{
						SpanID: model.SpanID(2),
					},
				},
			},
			expected: &model.Trace{
				Spans: []*model.Span{
					{
						SpanID:   model.SpanID(1),
						Duration: 1 * time.Microsecond,
						Tags: model.KeyValues{
							model.String(string(ext.SpanKind), string(ext.SpanKindRPCClientEnum)),
						},
					},
					{
						SpanID:   model.SpanID(1),
						Duration: 5 * time.Microsecond,
						Tags: model.KeyValues{
							model.String(string(ext.SpanKind), string(ext.SpanKindRPCClientEnum)),
						},
					},
					{
						SpanID:   model.SpanID(1),
						Duration: 2 * time.Microsecond,
						Tags: model.KeyValues{
							model.String(string(ext.SpanKind), string(ext.SpanKindRPCServerEnum)),
						},
					},
					{
						SpanID: model.SpanID(2),
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual, err := MergeSpans().Adjust(tt.input)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected.Warnings, actual.Warnings)
			assert.ElementsMatch(t, tt.expected.Spans, actual.Spans)
		})
	}

}
