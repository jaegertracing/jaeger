// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/otelsemconv"
)

var _ Adjuster = (*ResourceAttributesAdjuster)(nil)

var libraryKeys = map[string]struct{}{
	string(otelsemconv.TelemetrySDKLanguageKey):   {},
	string(otelsemconv.TelemetrySDKNameKey):       {},
	string(otelsemconv.TelemetrySDKVersionKey):    {},
	string(otelsemconv.TelemetryDistroNameKey):    {},
	string(otelsemconv.TelemetryDistroVersionKey): {},
}

// MoveLibraryAttributes creates an adjuster that moves the OpenTelemetry library
// attributes from spans to the parent resource so that the UI can
// display them separately under Process.
// https://github.com/jaegertracing/jaeger/issues/4534
func MoveLibraryAttributes() ResourceAttributesAdjuster {
	return ResourceAttributesAdjuster{}
}

type ResourceAttributesAdjuster struct{}

func (o ResourceAttributesAdjuster) Adjust(traces ptrace.Traces) {
	resourceSpans := traces.ResourceSpans()
	for i := 0; i < resourceSpans.Len(); i++ {
		rs := resourceSpans.At(i)
		resource := rs.Resource()
		scopeSpans := rs.ScopeSpans()
		for j := 0; j < scopeSpans.Len(); j++ {
			ss := scopeSpans.At(j)
			spans := ss.Spans()
			for k := 0; k < spans.Len(); k++ {
				span := spans.At(k)
				o.moveAttributes(span, resource)
			}
		}
	}
}

func (ResourceAttributesAdjuster) moveAttributes(span ptrace.Span, resource pcommon.Resource) {
	replace := make(map[string]pcommon.Value)
	span.Attributes().Range(func(k string, v pcommon.Value) bool {
		if _, ok := libraryKeys[k]; ok {
			replace[k] = v
		}
		return true
	})
	for k, v := range replace {
		existing, ok := resource.Attributes().Get(k)
		if ok && existing.AsRaw() != v.AsRaw() {
			jptrace.AddWarnings(span, "conflicting values between Span and Resource for attribute "+k)
			continue
		}
		v.CopyTo(resource.Attributes().PutEmpty(k))
		span.Attributes().Remove(k)
	}
}
