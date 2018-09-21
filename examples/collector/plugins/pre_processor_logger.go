package main

import (
	"fmt"

	"github.com/jaegertracing/jaeger/cmd/collector/app"
	"github.com/jaegertracing/jaeger/model"
)

var PreProcess app.ProcessSpans = func(spans []*model.Span) {
	fmt.Printf("PreProcessSpans...  %d spans\n", len(spans))
}
