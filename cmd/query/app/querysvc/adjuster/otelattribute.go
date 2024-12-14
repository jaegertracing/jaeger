// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"fmt"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/pkg/otelsemconv"
)

var otelLibraryKeys = map[string]struct{}{
	string(otelsemconv.TelemetrySDKLanguageKey):   {},
	string(otelsemconv.TelemetrySDKNameKey):       {},
	string(otelsemconv.TelemetrySDKVersionKey):    {},
	string(otelsemconv.TelemetryDistroNameKey):    {},
	string(otelsemconv.TelemetryDistroVersionKey): {},
}

// OTELAttribute creates an adjuster that removes the OpenTelemetry library attributes
// from spans and adds them to the attributes of a resource.
func OTELAttribute() Adjuster {
	return Func(func(traces ptrace.Traces) (ptrace.Traces, error) {
		adjuster := otelAttributeAdjuster{}
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
					adjuster.adjust(span, resource)
				}
			}
		}
		return traces, nil
	})
}

type otelAttributeAdjuster struct{}

func (otelAttributeAdjuster) adjust(span ptrace.Span, resource pcommon.Resource) {
	replace := make(map[string]pcommon.Value)
	span.Attributes().Range(func(k string, v pcommon.Value) bool {
		if _, ok := otelLibraryKeys[k]; ok {
			replace[k] = v
		}
		return true
	})
	for k, v := range replace {
		if existing, ok := resource.Attributes().Get(k); ok {
			if existing.AsRaw() != v.AsRaw() {
				span.Attributes().PutStr(adjusterWarningAttribute, fmt.Sprintf("conflicting attribute values for %s", k))
			}
		} else {
			v.CopyTo(resource.Attributes().PutEmpty(k))
		}
		span.Attributes().Remove(k)
	}
}
