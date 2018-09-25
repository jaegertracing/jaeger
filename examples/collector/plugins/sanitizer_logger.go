package main

import (
	"fmt"

	"github.com/jaegertracing/jaeger/cmd/collector/app/sanitizer"
	"github.com/jaegertracing/jaeger/model"
)

// Sanitizer is an example of a very simple plugin that only print some information
var Sanitizer sanitizer.SanitizeSpan = func(span *model.Span) *model.Span {
	fmt.Printf("Sanitizer... TraceID=%v SpanID=%v OperationName=%s\n", span.TraceID, span.SpanID, span.OperationName)
	return span
}
