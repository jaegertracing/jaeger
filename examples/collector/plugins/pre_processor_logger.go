package main

import (
	"github.com/jaegertracing/jaeger/cmd/collector/app"
	"github.com/jaegertracing/jaeger/model"
	"fmt"
)

func preProcessSpans(spans []*model.Span) {
	fmt.Printf("PreProcessSpans...  %d spans\n", len(spans))
}

var PreProcessSpans app.ProcessSpans = preProcessSpans
