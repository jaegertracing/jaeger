package main

import (
	"github.com/jaegertracing/jaeger/cmd/collector/app/sanitizer"
	"github.com/jaegertracing/jaeger/model"
	"fmt"
)

func sanitize(span *model.Span) *model.Span {
	fmt.Printf("Sanitizer... TraceID=%v SpanID=%v OperationName=%s\n", span.TraceID, span.SpanID, span.OperationName)
	return span
}

var Sanitizer sanitizer.SanitizeSpan = sanitize
