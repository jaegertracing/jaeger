package main

import (
	"github.com/jaegertracing/jaeger/model"
	"fmt"
	"github.com/jaegertracing/jaeger/cmd/collector/app"
)

func preSave(span *model.Span) {
	fmt.Printf("PreSave... TraceID=%v SpanID=%v OperationName=%s\n", span.TraceID, span.SpanID, span.OperationName)
}

var PreSave app.ProcessSpan = preSave
