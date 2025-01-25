// Copyright (c) 2022 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package sanitizer

import (
	"github.com/jaegertracing/jaeger-idl/model/v1"
)

const (
	serviceNameReplacement = "empty-service-name"
	nullProcessServiceName = "null-process-and-service-name"
)

// NewEmptyServiceNameSanitizer returns a function that replaces empty service name
// with a string "empty-service-name".
// If the whole span.Process is null, it creates one with "null-process-and-service-name".
func NewEmptyServiceNameSanitizer() SanitizeSpan {
	return sanitizeEmptyServiceName
}

// Sanitize sanitizes the service names in the span annotations.
func sanitizeEmptyServiceName(span *model.Span) *model.Span {
	if span.Process == nil {
		span.Process = &model.Process{ServiceName: nullProcessServiceName}
	} else if span.Process.ServiceName == "" {
		span.Process.ServiceName = serviceNameReplacement
	}
	return span
}
