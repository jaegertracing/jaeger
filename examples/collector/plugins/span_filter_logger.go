package main

import (
	"fmt"

	"github.com/jaegertracing/jaeger/cmd/collector/app"
	"github.com/jaegertracing/jaeger/model"
)

var SpanFilter app.FilterSpan = func(span *model.Span) bool {
	fmt.Printf("SpanFilter... TraceID=%v SpanID=%v OperationName=%s\n", span.TraceID, span.SpanID, span.OperationName)
	return true
}
