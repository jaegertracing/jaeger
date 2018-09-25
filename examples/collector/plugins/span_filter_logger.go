package main

import (
	"fmt"

	"github.com/jaegertracing/jaeger/cmd/collector/app"
	"github.com/jaegertracing/jaeger/model"
)

// SpanFilter is an example of a very simple plugin that only print some information
var SpanFilter app.FilterSpan = func(span *model.Span) bool {
	fmt.Printf("SpanFilter... TraceID=%v SpanID=%v OperationName=%s\n", span.TraceID, span.SpanID, span.OperationName)
	return true
}
