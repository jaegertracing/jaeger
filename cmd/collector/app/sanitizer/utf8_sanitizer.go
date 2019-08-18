// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sanitizer

import (
	"fmt"
	"unicode/utf8"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/jaegertracing/jaeger/model"
)

const (
	invalidOperation = "InvalidOperationName"
	invalidService   = "InvalidServiceName"
	invalidTagKey    = "InvalidTagKey"
)

// utf8Sanitizer sanitizes all strings in spans
type utf8Sanitizer struct {
	logger *zap.Logger
}

// NewUTF8Sanitizer creates a UTF8 sanitizer.
func NewUTF8Sanitizer(logger *zap.Logger) SanitizeSpan {
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

func (s *utf8Sanitizer) logSpan(span *model.Span, message string, field zapcore.Field) {
	s.logger.Info(
		message,
		zap.String("trace_id", span.TraceID.String()),
		zap.String("span_id", span.SpanID.String()), field)
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
