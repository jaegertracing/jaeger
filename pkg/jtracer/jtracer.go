// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jtracer

import (
	"context"
	"os"
	"strings"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	nooptrace "go.opentelemetry.io/otel/trace/noop"

	"github.com/jaegertracing/jaeger/internal/otelsemconv"
)

type JTracer struct {
	OTEL   trace.TracerProvider
	closer func(ctx context.Context) error
}

var once sync.Once

func New(serviceName string) (*JTracer, error) {
	return newHelper(serviceName, initOTEL)
}

func newHelper(
	serviceName string,
	tracerProvider func(ctx context.Context, svc string) (*sdktrace.TracerProvider, error),
) (*JTracer, error) {
	ctx := context.Background()
	provider, err := tracerProvider(ctx, serviceName)
	if err != nil {
		return nil, err
	}

	return &JTracer{
		OTEL: provider,
		closer: func(ctx context.Context) error {
			return provider.Shutdown(ctx)
		},
	}, nil
}

func NoOp() *JTracer {
	return &JTracer{
		OTEL: nooptrace.NewTracerProvider(),
	}
}

// initOTEL initializes OTEL Tracer
func initOTEL(ctx context.Context, svc string) (*sdktrace.TracerProvider, error) {
	return initHelper(ctx, svc, otelExporter, otelResource)
}

func initHelper(
	ctx context.Context,
	svc string,
	otelExporter func(_ context.Context) (sdktrace.SpanExporter, error),
	otelResource func(_ context.Context, _ /* svc */ string) (*resource.Resource, error),
) (*sdktrace.TracerProvider, error) {
	res, err := otelResource(ctx, svc)
	if err != nil {
		return nil, err
	}

	traceExporter, err := otelExporter(ctx)
	if err != nil {
		return nil, err
	}

	// Register the trace exporter with a TracerProvider, using a batch
	// span processor to aggregate spans before export.
	bsp := sdktrace.NewBatchSpanProcessor(traceExporter)

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

func otelResource(ctx context.Context, svc string) (*resource.Resource, error) {
	return resource.New(
		ctx,
		resource.WithSchemaURL(otelsemconv.SchemaURL),
		resource.WithAttributes(otelsemconv.ServiceNameKey.String(svc)),
		resource.WithTelemetrySDK(),
		resource.WithHost(),
		resource.WithOSType(),
		resource.WithFromEnv(),
	)
}

func defaultGRPCOptions() []otlptracegrpc.Option {
	var options []otlptracegrpc.Option
	if !(strings.HasPrefix(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"), "https://") || strings.ToLower(os.Getenv("OTEL_EXPORTER_OTLP_INSECURE")) == "false") {
		options = append(options, otlptracegrpc.WithInsecure())
	}
	return options
}

func otelExporter(ctx context.Context) (sdktrace.SpanExporter, error) {
	client := otlptracegrpc.NewClient(
		defaultGRPCOptions()...,
	)
	return otlptrace.New(ctx, client)
}

// Shutdown the tracerProvider to clean up resources
func (jt *JTracer) Close(ctx context.Context) error {
	if jt.closer != nil {
		return jt.closer(ctx)
	}
	return nil
}
