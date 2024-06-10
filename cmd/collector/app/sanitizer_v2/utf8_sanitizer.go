package sanitizer_v2

import (
	"fmt"
	"unicode/utf8"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

const (
	invalidOperation = "InvalidOperationName"
	invalidService   = "InvalidServiceName"
	invalidTagKey    = "InvalidTagKey"
)

// UTF8Sanitizer sanitizes all strings in spans.
type UTF8Sanitizer struct {
	logger *zap.Logger
}

// NewUTF8Sanitizer creates a UTF8 sanitizer with logging functionality.
func NewUTF8Sanitizer(logger *zap.Logger) SanitizeSpan {
	return UTF8Sanitizer{logger: logger}.Sanitize
}

// Sanitize sanitizes the UTF8 in the spans.
func (s UTF8Sanitizer) Sanitize(span ptrace.Span) ptrace.Span {
	if !utf8.ValidString(span.Name()) {
		s.logger.Info("Invalid utf8 operation name", zap.String("operation_name", span.Name()))
		span.SetName(invalidOperation)
	}

	attributes := span.Attributes()
	serviceNameAttr, ok := attributes.Get("service.name")
	if ok && !utf8.ValidString(serviceNameAttr.Str()) {
		s.logger.Info("Invalid utf8 service name", zap.String("service_name", serviceNameAttr.Str()))
		attributes.PutStr("service.name", invalidService)
	}

	sanitizeAttributes(attributes)
	return span
}

// sanitizeAttributes sanitizes attributes to ensure UTF8 validity.
func sanitizeAttributes(attributes pcommon.Map) {
	attributes.Range(func(k string, v pcommon.Value) bool {
		if v.Type() == pcommon.ValueTypeStr && !utf8.ValidString(v.Str()) {
			attributes.PutStr(k, fmt.Sprintf("%s:%s", k, v.Str()))
		}
		return true
	})
}
