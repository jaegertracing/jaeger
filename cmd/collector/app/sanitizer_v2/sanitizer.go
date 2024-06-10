package sanitizer_v2

import "go.opentelemetry.io/collector/pdata/ptrace"

// SanitizeSpan sanitizes/normalizes spans. Any business logic that needs to be applied to normalize the contents of a
// span should implement this interface.
type SanitizeSpan func(span ptrace.Span) ptrace.Span

// NewStandardSanitizers are automatically applied by SpanProcessor.
func NewStandardSanitizers() []SanitizeSpan {
	return []SanitizeSpan{
		NewEmptyServiceNameSanitizer(),
	}
}

// NewChainedSanitizer creates a Sanitizer from the variadic list of passed Sanitizers.
// If the list only has one element, it is returned directly to minimize indirection.
func NewChainedSanitizer(sanitizers ...SanitizeSpan) SanitizeSpan {
	if len(sanitizers) == 1 {
		return sanitizers[0]
	}
	return func(span ptrace.Span) ptrace.Span {
		for _, s := range sanitizers {
			span = s(span)
		}
		return span
	}
}
