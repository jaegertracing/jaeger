// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package sanitizer

import (
	"fmt"

	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/pkg/otelsemconv"
)

const (
	emptyServiceName   = "empty-service-name"
	missingServiceName = "missing-service-name"
)

// NewEmptyServiceNameSanitizer returns a sanitizer function that replaces
// empty and missing service names with placeholder strings.
func NewEmptyServiceNameSanitizer() Sanitize {
	return sanitizeEmptyServiceName
}

func sanitizeEmptyServiceName(traces ptrace.Traces) ptrace.Traces {
	resourceSpans := traces.ResourceSpans()
	for i := 0; i < resourceSpans.Len(); i++ {
		resourceSpan := resourceSpans.At(i)
		attributes := resourceSpan.Resource().Attributes()
		serviceName, ok := attributes.Get(string(otelsemconv.ServiceNameKey))
		fmt.Printf("service name: %s, ok: %v\n", serviceName.Str(), ok)
		if !ok {
			attributes.PutStr(string(otelsemconv.ServiceNameKey), missingServiceName)
		} else if serviceName.Str() == "" {
			fmt.Println("here")
			attributes.PutStr(string(otelsemconv.ServiceNameKey), emptyServiceName)
		}
	}
	return traces
}
