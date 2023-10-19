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
	"fmt"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
)

type JTracer struct {
	OTEL   trace.TracerProvider
	closer func(ctx context.Context) error
}

var once sync.Once

func New(serviceName string) (*JTracer, error) {
	ctx := context.Background()
	tracerProvider, err := initOTEL(ctx, serviceName)
	if err != nil {
		return nil, err
	}

	closer := func(ctx context.Context) error {
		return tracerProvider.Shutdown(ctx)
	}

	return &JTracer{
		OTEL:   tracerProvider,
		closer: closer,
	}, nil
}

func NoOp() *JTracer {
	return &JTracer{
		OTEL: trace.NewNoopTracerProvider(),
		closer: func(ctx context.Context) error {
			return nil
		},
	}
}

// initOTEL initializes OTEL Tracer
func initOTEL(ctx context.Context, svc string) (*sdktrace.TracerProvider, error) {
	traceExporter, err := otelExporter(ctx)
	if err != nil {
		return nil, err
	}

	// Register the trace exporter with a TracerProvider, using a batch
	// span processor to aggregate spans before export.
	bsp := sdktrace.NewBatchSpanProcessor(traceExporter)

	res, err := resource.New(
		ctx,
		resource.WithSchemaURL(semconv.SchemaURL),
		resource.WithAttributes(semconv.ServiceNameKey.String(svc)),
		resource.WithTelemetrySDK(),
		resource.WithHost(),
		resource.WithOSType(),
	)
	if err != nil {
		return nil, err
	}

	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(bsp),
		sdktrace.WithResource(res),
	)

	once.Do(func() {
		otel.SetTextMapPropagator(
			propagation.NewCompositeTextMapPropagator(
				propagation.TraceContext{},
				propagation.Baggage{},
			))
	})

	otel.SetTracerProvider(tracerProvider)

	return tracerProvider, nil
}

func otelExporter(ctx context.Context) (sdktrace.SpanExporter, error) {
	client := otlptracegrpc.NewClient(
		otlptracegrpc.WithInsecure(),
	)
	traceExporter, err := otlptrace.New(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("failed to create trace exporter: %w", err)
	}

	return traceExporter, nil
}

// Shutdown the tracerProvider to clean up resources
func (jt *JTracer) Close(ctx context.Context) error {
	if jt.closer != nil {
		return jt.closer(ctx)
	}
	return nil
}
