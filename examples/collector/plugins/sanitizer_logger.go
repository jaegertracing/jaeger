package main

import (
	"fmt"

	"github.com/jaegertracing/jaeger/cmd/collector/app/sanitizer"
	"github.com/jaegertracing/jaeger/model"
)

var Sanitizer sanitizer.SanitizeSpan = func(span *model.Span) *model.Span {
	fmt.Printf("Sanitizer... TraceID=%v SpanID=%v OperationName=%s\n", span.TraceID, span.SpanID, span.OperationName)
	return span
}
