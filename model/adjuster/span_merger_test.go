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

	"github.com/opentracing/opentracing-go/ext"
	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/model"
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
			name: "duplicate Jaeger spans: select last span by Incomplete flag and merge logs and tags",
			input: &model.Trace{
				Spans: []*model.Span{
					{
						SpanID: model.SpanID(1),
						Logs: []model.Log{
							{
								Timestamp: time.Date(2018, 11, 14, 23, 0, 0, 0, time.UTC),
								Fields:    model.KeyValues{model.String("message", "testlog")},
							},
						},
						Incomplete: true,
					},
					{
						SpanID:     model.SpanID(1),
						Duration:   1 * time.Microsecond,
						Tags:       model.KeyValues{model.String("t1", "teststring")},
						Incomplete: true,
					},
					{
						SpanID:     model.SpanID(1),
						Duration:   5 * time.Microsecond,
						Tags:       model.KeyValues{model.String("t2", "teststring2")},
						Incomplete: true,
					},
					{
						SpanID:   model.SpanID(1),
						Duration: 5 * time.Microsecond,
						Logs: []model.Log{
							{
								Timestamp: time.Date(2018, 11, 14, 23, 0, 5, 0, time.UTC),
								Fields:    model.KeyValues{model.String("message", "testlog")},
							},
						},
						Incomplete: true,
					},
					{
						SpanID:   model.SpanID(1),
						Duration: 8 * time.Microsecond,
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
						Duration: 8 * time.Microsecond,
						Tags:     model.KeyValues{model.String("t1", "teststring"), model.String("t2", "teststring2")},
						Logs: []model.Log{
							{
								Timestamp: time.Date(2018, 11, 14, 23, 0, 0, 0, time.UTC),
								Fields:    model.KeyValues{model.String("message", "testlog")},
							},
							{
								Timestamp: time.Date(2018, 11, 14, 23, 0, 5, 0, time.UTC),
								Fields:    model.KeyValues{model.String("message", "testlog")},
							},
						},
					},
					{
						SpanID: model.SpanID(2),
					},
				},
			},
		},
		{
			name: "duplicate Jaeger spans: select last span by Incomplete flag and merge references and warnings",
			input: &model.Trace{
				Spans: []*model.Span{
					{
						SpanID:     model.SpanID(1),
						Incomplete: true,
					},
					{
						SpanID: model.SpanID(1),
						References: []model.SpanRef{
							{
								RefType: model.SpanRefType_CHILD_OF,
								TraceID: model.NewTraceID(2, 3),
								SpanID:  model.SpanID(3),
							},
						},
						Duration:   1 * time.Microsecond,
						Incomplete: true,
						Warnings:   []string{"First Warning", "Second Warning"},
					},
					{
						SpanID:     model.SpanID(1),
						Duration:   5 * time.Microsecond,
						Incomplete: true,
						References: []model.SpanRef{
							{
								RefType: model.SpanRefType_CHILD_OF,
								TraceID: model.NewTraceID(2, 3),
								SpanID:  model.SpanID(4),
							},
						},
					},
					{
						SpanID:     model.SpanID(1),
						Duration:   5 * time.Microsecond,
						Warnings:   []string{"Third Warning"},
						Incomplete: true,
					},
					{
						SpanID:   model.SpanID(1),
						Duration: 8 * time.Microsecond,
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
						Duration: 8 * time.Microsecond,
						References: []model.SpanRef{
							{
								RefType: model.SpanRefType_CHILD_OF,
								TraceID: model.NewTraceID(2, 3),
								SpanID:  model.SpanID(3),
							},
							{
								RefType: model.SpanRefType_CHILD_OF,
								TraceID: model.NewTraceID(2, 3),
								SpanID:  model.SpanID(4),
							},
						},
						Warnings: []string{"First Warning", "Second Warning", "Third Warning"},
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
