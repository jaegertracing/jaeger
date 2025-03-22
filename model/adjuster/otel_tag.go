// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/otelsemconv"
)

var otelLibraryKeys = map[string]struct{}{
	string(otelsemconv.TelemetrySDKLanguageKey):   {},
	string(otelsemconv.TelemetrySDKNameKey):       {},
	string(otelsemconv.TelemetrySDKVersionKey):    {},
	string(otelsemconv.TelemetryDistroNameKey):    {},
	string(otelsemconv.TelemetryDistroVersionKey): {},
}

func OTelTagAdjuster() Adjuster {
	adjustSpanTags := func(span *model.Span) {
		newI := 0
		for i, tag := range span.Tags {
			if _, ok := otelLibraryKeys[tag.Key]; ok {
				span.Process.Tags = append(span.Process.Tags, tag)
				continue
			}
			if i != newI {
				span.Tags[newI] = tag
			}
			newI++
		}
		span.Tags = span.Tags[:newI]
	}

	return Func(func(trace *model.Trace) {
		for _, span := range trace.Spans {
			adjustSpanTags(span)
			model.KeyValues(span.Process.Tags).Sort()
		}
	})
}
