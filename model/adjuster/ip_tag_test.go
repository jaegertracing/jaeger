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

func TestIPTagAdjuster(t *testing.T) {
	trace := &model.Trace{
		Spans: []*model.Span{
			{
				Tags: model.KeyValues{
					model.Int64("a", 42),
					model.String("ip", "not integer"),
					model.Int64("ip", 1<<24|2<<16|3<<8|4),
					model.String("peer.ipv4", "not integer"),
					model.Int64("peer.ipv4", 1<<24|2<<16|3<<8|4),
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
		model.String("peer.ipv4", "not integer"),
		model.String("peer.ipv4", "1.2.3.4"),
	}
	assert.Equal(t, expectedSpanTags, trace.Spans[0].Tags)

	expectedProcessTags := model.KeyValues{
		model.Int64("a", 42),
		model.String("ip", "1.2.3.4"), // sorted
		model.String("ip", "not integer"),
	}
	assert.Equal(t, expectedProcessTags, trace.Spans[0].Process.Tags)
}
