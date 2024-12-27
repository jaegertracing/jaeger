// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/model"
)

func TestIPTagAdjuster(t *testing.T) {
	trace := &model.Trace{
		Spans: []*model.Span{
			{
				Tags: model.KeyValues{
					model.Int64("a", 42),
					model.String("ip", "not integer"),
					model.Int64("ip", 1<<24|2<<16|3<<8|4),
					model.Float64("ip", 1<<24|2<<16|3<<8|4),
					model.String("peer.ipv4", "not integer"),
					model.Int64("peer.ipv4", 1<<24|2<<16|3<<8|4),
					model.Float64("peer.ipv4", 1<<24|2<<16|3<<8|4),
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
	trace = IPTagAdjuster().Adjust(trace)

	expectedSpanTags := model.KeyValues{
		model.Int64("a", 42),
		model.String("ip", "not integer"),
		model.String("ip", "1.2.3.4"),
		model.String("ip", "1.2.3.4"),
		model.String("peer.ipv4", "not integer"),
		model.String("peer.ipv4", "1.2.3.4"),
		model.String("peer.ipv4", "1.2.3.4"),
	}
	assert.Equal(t, expectedSpanTags, model.KeyValues(trace.Spans[0].Tags))

	expectedProcessTags := model.KeyValues{
		model.Int64("a", 42),
		model.String("ip", "1.2.3.4"), // sorted
		model.String("ip", "not integer"),
	}
	assert.Equal(t, expectedProcessTags, model.KeyValues(trace.Spans[0].Process.Tags))
}
