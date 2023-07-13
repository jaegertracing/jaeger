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

	"github.com/go-logr/zapr"
	"github.com/opentracing/opentracing-go"
	"go.opentelemetry.io/otel"
	otbridge "go.opentelemetry.io/otel/bridge/opentracing"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type JTracer struct {
	OT     opentracing.Tracer
	OTEL   trace.TracerProvider
	log    *zap.Logger
	closer func() error
}

var once sync.Once

func New() JTracer {
	jt := JTracer{}
	opentracingTracer, otelTracerProvider, logger, closed := jt.initBoth()

	return JTracer{
		OT:     opentracingTracer,
		OTEL:   otelTracerProvider,
		log:    logger,
		closer: closed,
	}
}

func NoOp() JTracer {
	return JTracer{OT: opentracing.NoopTracer{}, OTEL: trace.NewNoopTracerProvider()}
}

// initBoth initializes OpenTelemetry SDK and uses OTel-OpenTracing Bridge
func (jt JTracer) initBoth() (opentracing.Tracer, trace.TracerProvider, *zap.Logger, func() error) {
	zc := zap.NewDevelopmentConfig()
	zc.Level = zap.NewAtomicLevelAt(zapcore.Level(-8)) // level used by OTEL's Debug()
	logger, err := zc.Build()
	if err != nil {
		panic(err)
	}
	otel.SetLogger(zapr.NewLogger(logger))

	traceExporter, err := otelExporter()
	if err != nil {
		logger.Sugar().Fatalf("failed to create exporter", zap.Any("error", err))
	}

	// Register the trace exporter with a TracerProvider, using a batch
	// span processor to aggregate spans before export.
	bsp := sdktrace.NewBatchSpanProcessor(traceExporter, sdktrace.WithBlocking())
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

	closer := func() error {
		return tracerProvider.Shutdown(context.Background())
	}

	return otTracer, wrapperTracerProvider, logger, closer
}

func otelExporter() (sdktrace.SpanExporter, error) {
	client := otlptracegrpc.NewClient(
		otlptracegrpc.WithInsecure(),
	)
	traceExporter, err := otlptrace.New(context.Background(), client)
	if err != nil {
		return nil, fmt.Errorf("failed to create trace exporter: %w", err)
	}

	return traceExporter, nil
}

// Shutdown the tracerProvider to clean up resources
func (jt JTracer) Close(ctx context.Context) error {
	return jt.closer()
}
