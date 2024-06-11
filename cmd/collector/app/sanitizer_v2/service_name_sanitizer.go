// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package sanitizer_v2

import (
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// Cache interface similar to the one in V1
type Cache interface {
	Get(alias string) string
	IsEmpty() bool
}

// serviceNameSanitizer sanitizes the service names in span annotations given a source of truth alias to service cache.
type serviceNameSanitizer struct {
	cache Cache
}

// Sanitize sanitizes the service names in the span annotations.
func (s serviceNameSanitizer) Sanitize(traces ptrace.Traces) ptrace.Traces {
	if s.cache.IsEmpty() {
		return traces
	}

	resourceSpans := traces.ResourceSpans()
	for i := 0; i < resourceSpans.Len(); i++ {
		resourceSpan := resourceSpans.At(i)
		attributes := resourceSpan.Resource().Attributes()
		serviceNameAttr, exists := attributes.Get("service.name")
		if !exists || serviceNameAttr.Type() != pcommon.ValueTypeStr {
			continue
		}

		alias := serviceNameAttr.Str()
		serviceName := s.cache.Get(alias)
		if serviceName != "" {
			attributes.PutStr("service.name", serviceName)
		}
	}

	return traces
}
