// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package sanitizer

import (
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/telemetry/otelsemconv"
)

const (
	emptyServiceName     = "empty-service-name"
	serviceNameWrongType = "service-name-wrong-type"
	missingServiceName   = "missing-service-name"
)

// NewEmptyServiceNameSanitizer returns a sanitizer function that replaces
// empty and missing service names with placeholder strings.
func NewEmptyServiceNameSanitizer() Func {
	return sanitizeEmptyServiceName
}

func sanitizeEmptyServiceName(traces ptrace.Traces) ptrace.Traces {
	resourceSpans := traces.ResourceSpans()

	var index []int
	for i := 0; i < resourceSpans.Len(); i++ {
		resourceSpan := resourceSpans.At(i)
		attributes := resourceSpan.Resource().Attributes()
		serviceName, ok := attributes.Get(string(otelsemconv.ServiceNameKey))
		if !ok || serviceName.Str() == "" || serviceName.Type() != pcommon.ValueTypeStr {
			index = append(index, i)
		}
	}

	if len(index) == 0 {
		return traces
	}

	// Make sure exp.sanitizer func work when multiple exporters are defined in pipeline
	// More info see: https://github.com/open-telemetry/opentelemetry-collector/blob/main/internal/fanoutconsumer/traces.go#L66-L75
	newTraces := ptrace.NewTraces()
	traces.CopyTo(newTraces)
	rss := newTraces.ResourceSpans()
	for _, v := range index {
		attributes := rss.At(v).Resource().Attributes()
		serviceName, ok := attributes.Get(string(otelsemconv.ServiceNameKey))
		switch {
		case !ok:
			attributes.PutStr(string(otelsemconv.ServiceNameKey), missingServiceName)
		case serviceName.Type() != pcommon.ValueTypeStr:
			attributes.PutStr(string(otelsemconv.ServiceNameKey), serviceNameWrongType)
		case serviceName.Str() == "":
			attributes.PutStr(string(otelsemconv.ServiceNameKey), emptyServiceName)
		}
	}

	return newTraces
}
