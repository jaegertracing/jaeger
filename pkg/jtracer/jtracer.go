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
	"log"
	"net"
	"sync"
	"time"

	"github.com/opentracing/opentracing-go"
	"go.opentelemetry.io/otel"
	otbridge "go.opentelemetry.io/otel/bridge/opentracing"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type JTracer struct {
	OT   opentracing.Tracer
	OTEL *sdktrace.TracerProvider
	// logger *zap.Logger
}

var once sync.Once

func New() JTracer {
	opentracingTracer, otelTracerProvider := initBoth()

	// Shutdown the tracerProvider to clean up resources
	defer func() {
		if err := otelTracerProvider.Shutdown(context.Background()); err != nil {
			log.Fatal("Error shutting down tracer provider: %w", err)
		}
	}()
	return JTracer{OT: opentracingTracer, OTEL: otelTracerProvider}
}

func NoOp() JTracer {
	return JTracer{OT: opentracing.NoopTracer{}, OTEL: &sdktrace.TracerProvider{}}
}

// initBoth initializes OpenTelemetry SDK and uses OTel-OpenTracing Bridge
func initBoth() (opentracing.Tracer, *sdktrace.TracerProvider) {
	ctx := context.Background()
	traceExporter, err := newExporter(ctx)
	if err != nil {
		log.Fatal("failed to create exporter", zap.Any("error", err))
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
	otTracer, _ := otbridge.NewTracerPair(tracerProvider.Tracer(""))

	return otTracer, tracerProvider
}

// newExporter returns a console exporter.
func newExporter(ctx context.Context) (sdktrace.SpanExporter, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	server := grpc.NewServer()

	lis, _ := net.Listen("tcp", ":0")
	go func() {
		err := server.Serve(lis)
		log.Fatal("failed to create gRPC server connection: %w", err)

	}()
	conn, err := grpc.DialContext(ctx, lis.Addr().String(),
		// Note the use of insecure transport here. TLS is recommended in production.
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC connection to collector: %w", err)
	}

	// Set up a trace exporter
	traceExporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn))
	if err != nil {
		return nil, fmt.Errorf("failed to create trace exporter: %w", err)
	}
	return traceExporter, nil
}
