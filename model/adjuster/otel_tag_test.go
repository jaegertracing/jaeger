// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/otelsemconv"
)

func TestOTelTagAdjuster(t *testing.T) {
	testCases := []struct {
		description string
		span        *model.Span
		expected    *model.Span
	}{
		{
			description: "span with otel library tags",
			span: &model.Span{
				Tags: model.KeyValues{
					model.String("random_key", "random_value"),
					model.String(string(otelsemconv.TelemetrySDKLanguageKey), "Go"),
					model.String(string(otelsemconv.TelemetrySDKNameKey), "opentelemetry"),
					model.String(string(otelsemconv.TelemetrySDKVersionKey), "1.27.0"),
					// distro attrs intentionally after SDK attrs to test sorting
					model.String(string(otelsemconv.TelemetryDistroNameKey), "opentelemetry"),
					model.String(string(otelsemconv.TelemetryDistroVersionKey), "blah"),
					model.String("another_key", "another_value"),
				},
				Process: &model.Process{
					Tags: model.KeyValues{},
				},
			},
			expected: &model.Span{
				Tags: model.KeyValues{
					model.String("random_key", "random_value"),
					model.String("another_key", "another_value"),
				},
				Process: &model.Process{
					Tags: model.KeyValues{
						model.String(string(otelsemconv.TelemetryDistroNameKey), "opentelemetry"),
						model.String(string(otelsemconv.TelemetryDistroVersionKey), "blah"),
						model.String(string(otelsemconv.TelemetrySDKLanguageKey), "Go"),
						model.String(string(otelsemconv.TelemetrySDKNameKey), "opentelemetry"),
						model.String(string(otelsemconv.TelemetrySDKVersionKey), "1.27.0"),
					},
				},
			},
		},
		{
			description: "span without otel library tags",
			span: &model.Span{
				Tags: model.KeyValues{
					model.String("random_key", "random_value"),
				},
				Process: &model.Process{
					Tags: model.KeyValues{},
				},
			},
			expected: &model.Span{
				Tags: model.KeyValues{
					model.String("random_key", "random_value"),
				},
				Process: &model.Process{
					Tags: model.KeyValues{},
				},
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.description, func(t *testing.T) {
			beforeTags := testCase.span.Tags

			trace := &model.Trace{
				Spans: []*model.Span{testCase.span},
			}
			trace = OTelTagAdjuster().Adjust(trace)
			assert.Equal(t, testCase.expected.Tags, trace.Spans[0].Tags)
			assert.Equal(t, testCase.expected.Process.Tags, trace.Spans[0].Process.Tags)

			newTag := model.String("new_key", "new_value")
			beforeTags[0] = newTag
			assert.Equal(t, newTag, testCase.span.Tags[0], "span.Tags still points to the same underlying array")
		})
	}
}
