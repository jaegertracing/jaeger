// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/model"
)

func TestSortTagsAndLogFieldsDoesSortFields(t *testing.T) {
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
		SortTagsAndLogFields().Adjust(trace)
		assert.Equal(t, testCase.expected, model.KeyValues(trace.Spans[0].Logs[0].Fields))
	}
}

func TestSortTagsAndLogFieldsDoesSortTags(t *testing.T) {
	testCases := []model.KeyValues{
		{
			model.String("http.method", http.MethodGet),
			model.String("http.url", "http://wikipedia.org"),
			model.Int64("http.status_code", 200),
			model.String("guid:x-request-id", "f61defd2-7a77-11ef-b54f-4fbb67a6d181"),
		},
		{
			// empty
		},
		{
			model.String("argv", "mkfs.ext4 /dev/sda1"),
		},
	}
	for _, tags := range testCases {
		trace := &model.Trace{
			Spans: []*model.Span{
				{
					Tags: tags,
				},
			},
		}
		SortTagsAndLogFields().Adjust(trace)
		assert.ElementsMatch(t, tags, trace.Spans[0].Tags)
		adjustedKeys := make([]string, len(trace.Spans[0].Tags))
		for i, kv := range trace.Spans[0].Tags {
			adjustedKeys[i] = kv.Key
		}
		assert.IsIncreasing(t, adjustedKeys)
	}
}

func TestSortTagsAndLogFieldsDoesSortProcessTags(t *testing.T) {
	trace := &model.Trace{
		Spans: []*model.Span{
			{
				Process: &model.Process{
					Tags: model.KeyValues{
						model.String("process.argv", "mkfs.ext4 /dev/sda1"),
						model.Int64("process.pid", 42),
						model.Int64("process.uid", 0),
						model.String("k8s.node.roles", "control-plane,etcd,master"),
					},
				},
			},
		},
	}
	SortTagsAndLogFields().Adjust(trace)
	adjustedKeys := make([]string, len(trace.Spans[0].Process.Tags))
	for i, kv := range trace.Spans[0].Process.Tags {
		adjustedKeys[i] = kv.Key
	}
	assert.IsIncreasing(t, adjustedKeys)
}

func TestSortTagsAndLogFieldsHandlesNilProcessTags(t *testing.T) {
	trace := &model.Trace{
		Spans: []*model.Span{
			{},
		},
	}
	SortTagsAndLogFields().Adjust(trace)
	require.Equal(t, &model.Trace{
		Spans: []*model.Span{
			{},
		},
	}, trace)
}
