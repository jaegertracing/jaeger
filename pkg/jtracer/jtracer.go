// Copyright (c) 2023 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package jtracer

import (
	"context"
	"log"
	"sync"

	"github.com/opentracing/opentracing-go"
	"go.opentelemetry.io/otel"
	otbridge "go.opentelemetry.io/otel/bridge/opentracing"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
)

type JTracer struct {
	OT   opentracing.Tracer
	OTEL trace.TracerProvider
}

var once sync.Once

func New() JTracer {
	opentracingTracer, otelTracerProvider := initBoth()
	return JTracer{OT: opentracingTracer, OTEL: otelTracerProvider}
}

func NoOp() JTracer {
	return JTracer{OT: opentracing.NoopTracer{}, OTEL: trace.NewNoopTracerProvider()}
}

// initBoth initializes OpenTelemetry SDK and uses OTel-OpenTracing Bridge
func initBoth() (opentracing.Tracer, trace.TracerProvider) {
	opts := []otlptracehttp.Option{otlptracehttp.WithInsecure()}
	traceExporter, err := otlptrace.New(
		context.Background(),
		otlptracehttp.NewClient(opts...),
	)
	if err != nil {
		log.Fatal("failed to create exporter: %w", err)
	}

	// Register the trace exporter with a TracerProvider, using a batch
	// span processor to aggregate spans before export.
	bsp := sdktrace.NewBatchSpanProcessor(traceExporter)
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(bsp),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String("otlp"),
		)),
	)

	once.Do(func() {
		otel.SetTextMapPropagator(
			propagation.NewCompositeTextMapPropagator(
				propagation.TraceContext{},
				propagation.Baggage{},
			))
	})

	// Use the bridgeTracer as your OpenTracing tracer(otTrace).
	otTracer, wrapperTracerProvider := otbridge.NewTracerPair(tracerProvider.Tracer(""))

	// Shutdown the tracerProvider to clean up resources
	err = tracerProvider.Shutdown(context.Background())
	if err != nil {
		log.Fatal("failed to shutdown tracerProvider: ", err)
	}

	return otTracer, wrapperTracerProvider
}
