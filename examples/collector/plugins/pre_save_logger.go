package main

import (
	"fmt"

	"github.com/jaegertracing/jaeger/cmd/collector/app"
	"github.com/jaegertracing/jaeger/model"
)

var PreSave app.ProcessSpan = func(span *model.Span) {
	fmt.Printf("PreSave... TraceID=%v SpanID=%v OperationName=%s\n", span.TraceID, span.SpanID, span.OperationName)
}
