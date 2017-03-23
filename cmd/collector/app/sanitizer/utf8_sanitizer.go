// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package sanitizer

import (
	"fmt"
	"unicode/utf8"

	"github.com/uber-go/zap"
	"github.com/uber/jaeger/model"
)

const (
	invalidOperation = "InvalidOperationName"
	invalidService   = "InvalidServiceName"
	invalidTagKey    = "InvalidTagKey"
)

// utf8Sanitizer sanitizes all strings in spans
type utf8Sanitizer struct {
	logger zap.Logger
}

// NewUTF8Sanitizer creates a UTF8 sanitizer.
func NewUTF8Sanitizer(logger zap.Logger) SanitizeSpan {
	utf8Sanitizer := utf8Sanitizer{logger: logger}
	return utf8Sanitizer.Sanitize
}

// Sanitize sanitizes the UTF8 in the spans.
func (s *utf8Sanitizer) Sanitize(span *model.Span) *model.Span {
	if !utf8.ValidString(span.OperationName) {
		s.logSpan(span, "Invalid utf8 operation name", zap.String("operation_name", span.OperationName))
		span.Tags = append(span.Tags, model.Binary(invalidOperation, []byte(span.OperationName)))
		span.OperationName = invalidOperation
	}
	if !utf8.ValidString(span.Process.ServiceName) {
		s.logSpan(span, "Invalid utf8 service name", zap.String("service_name", span.Process.ServiceName))
		span.Tags = append(span.Tags, model.Binary(invalidService, []byte(span.Process.ServiceName)))
		span.Process.ServiceName = invalidService
	}
	sanitizeKV(span.Process.Tags)
	sanitizeKV(span.Tags)
	for _, log := range span.Logs {
		sanitizeKV(log.Fields)
	}
	return span
}

func (s *utf8Sanitizer) logSpan(span *model.Span, message string, field zap.Field) {
	s.logger.Info(
		message,
		zap.String("traceId", span.TraceID.String()),
		zap.String("spanID", span.SpanID.String()), field)
}

func sanitizeKV(keyValues model.KeyValues) {
	for i, kv := range keyValues {
		if !utf8.ValidString(kv.Key) {
			keyValues[i] = model.Binary(invalidTagKey, []byte(fmt.Sprintf("%s:%s", kv.Key, kv.AsString())))
		} else if kv.VType == model.StringType && !utf8.ValidString(kv.VStr) {
			keyValues[i] = model.Binary(kv.Key, []byte(kv.VStr))
		}
	}
}
