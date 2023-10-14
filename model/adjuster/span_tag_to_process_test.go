// Copyright (c) 2019 The Jaeger Authors.
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

	"github.com/jaegertracing/jaeger/model"
)

func TestSpanTagsToProcessAdjuster(t *testing.T) {
	trace := &model.Trace{
		Spans: []*model.Span{
			{
				Tags: model.KeyValues{
					model.Int64("a", 42),
					model.String("b", "val"),
					model.Float64("c", 6.9),
				},
				Process: &model.Process{
					Tags: model.KeyValues{
						model.Int64("a", 42),
						model.String("b", "val"),
						model.Float64("c", 6.9),
					},
				},
			},
		},
	}
	out, _ := SpanTagsToProcessAdjuster().Adjust(trace)
	assert.Equal(t, trace, out)

	otelLibNameTrace := &model.Trace{
		Spans: []*model.Span{
			{
				Tags: model.KeyValues{
					model.Int64("a", 42),
					model.String("otel.library.name", "val"),
					model.Float64("c", 6.9),
				},
				Process: &model.Process{
					Tags: model.KeyValues{
						model.Int64("a", 42),
						model.Float64("c", 6.9),
					},
				},
			},
		},
	}
	outAdjusted, _ := SpanTagsToProcessAdjuster().Adjust(otelLibNameTrace)

	assert.Equal(t, "otel.library.name", outAdjusted.Spans[0].Process.Tags[2].Key)
}
