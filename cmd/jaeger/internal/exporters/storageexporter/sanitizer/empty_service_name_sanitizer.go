// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package sanitizer

import (
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// Constants for the replacement names
const (
	emptyServiceName   = "empty-service-name"
	missingServiceName = "missing-service-name"
)

// NewEmptyServiceNameSanitizer returns a function that replaces empty service names
// with a predefined string.
func NewEmptyServiceNameSanitizer() SanitizeTraces {
	return sanitizeEmptyServiceName
}

// sanitizeEmptyServiceName sanitizes the service names in the resource attributes.
func sanitizeEmptyServiceName(traces ptrace.Traces) ptrace.Traces {
	resourceSpans := traces.ResourceSpans()
	for i := 0; i < resourceSpans.Len(); i++ {
		resourceSpan := resourceSpans.At(i)
		attributes := resourceSpan.Resource().Attributes()
		serviceNameAttr, ok := attributes.Get("service.name")
		if !ok {
			// If service.name is missing, set it to nullProcessServiceName
			attributes.PutStr("service.name", missingServiceName)
		} else if serviceNameAttr.Str() == "" {
			// If service.name is empty, replace it with serviceNameReplacement
			attributes.PutStr("service.name", emptyServiceName)
		}
	}
	return traces
}
