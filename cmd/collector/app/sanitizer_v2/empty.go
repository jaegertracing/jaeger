package sanitizer_v2

import (
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// Constants for the replacement names
const (
	serviceNameReplacement = "empty-service-name"
	nullProcessServiceName = "null-process-and-service-name"
)

// NewEmptyServiceNameSanitizer returns a function that replaces empty service names
// with a predefined string.
func NewEmptyServiceNameSanitizer() SanitizeSpan {
	return sanitizeEmptyServiceName
}

// sanitizeEmptyServiceName sanitizes the service names in the span attributes.
func sanitizeEmptyServiceName(span ptrace.Span) ptrace.Span {
	attributes := span.Attributes()
	serviceNameAttr, ok := attributes.Get("service.name")

	if !ok {
		// If service.name is missing, set it to nullProcessServiceName
		attributes.PutStr("service.name", nullProcessServiceName)
	} else if serviceNameAttr.Str() == "" {
		// If service.name is empty, replace it with serviceNameReplacement
		attributes.PutStr("service.name", serviceNameReplacement)
	}

	return span
}
