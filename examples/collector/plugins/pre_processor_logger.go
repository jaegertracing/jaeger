package main

import (
	"fmt"

	"github.com/jaegertracing/jaeger/cmd/collector/app"
	"github.com/jaegertracing/jaeger/model"
)

func preProcessSpans(spans []*model.Span) {
	fmt.Printf("PreProcessSpans...  %d spans\n", len(spans))
}

var PreProcess app.ProcessSpans = preProcessSpans
