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

// NewServiceNameSanitizer creates a service name sanitizer with a given cache.
func NewServiceNameSanitizer(cache Cache) SanitizeSpan {
	sanitizer := serviceNameSanitizer{cache: cache}
	return sanitizer.Sanitize
}

// serviceNameSanitizer sanitizes the service names in span annotations given a source of truth alias to service cache.
type serviceNameSanitizer struct {
	cache Cache
}

// Sanitize sanitizes the service names in the span annotations.
func (s serviceNameSanitizer) Sanitize(span ptrace.Span) ptrace.Span {
	if s.cache.IsEmpty() {
		return span
	}

	attributes := span.Attributes()
	serviceNameAttr, exists := attributes.Get("service.name")
	if !exists || serviceNameAttr.Type() != pcommon.ValueTypeStr {
		return span
	}

	alias := serviceNameAttr.Str()
	serviceName := s.cache.Get(alias)
	if serviceName != "" {
		attributes.PutStr("service.name", serviceName)
	}

	return span
}
