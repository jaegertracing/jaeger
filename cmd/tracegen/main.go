// Copyright (c) 2018 The Jaeger Authors.
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

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"

	"github.com/go-logr/zapr"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.20.0"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/jaegertracing/jaeger/internal/jaegerclientenv2otel"
	"github.com/jaegertracing/jaeger/internal/tracegen"
)

func main() {
	zc := zap.NewDevelopmentConfig()
	zc.Level = zap.NewAtomicLevelAt(zapcore.Level(-8)) // level used by OTEL's Debug()
	logger, err := zc.Build()
	if err != nil {
		panic(err)
	}
	otel.SetLogger(zapr.NewLogger(logger))

	fs := flag.CommandLine
	cfg := new(tracegen.Config)
	cfg.Flags(fs)
	flag.Parse()

	otel.SetTextMapPropagator(propagation.TraceContext{})
	jaegerclientenv2otel.MapJaegerToOtelEnvVars(logger)

	tracers, shutdown := createTracers(cfg, logger)
	defer shutdown(context.Background())

	tracegen.Run(cfg, tracers, logger)
}

func createTracers(cfg *tracegen.Config, logger *zap.Logger) ([]trace.Tracer, func(context.Context) error) {
	if cfg.Services < 1 {
		cfg.Services = 1
	}
	var shutdown []func(context.Context) error
	var tracers []trace.Tracer
	for s := 0; s < cfg.Services; s++ {
		svc := cfg.Service
		if cfg.Services > 1 {
			svc = fmt.Sprintf("%s-%02d", svc, s)
		}

		exp, err := createOtelExporter(cfg.TraceExporter)
		if err != nil {
			logger.Sugar().Fatalf("cannot create trace exporter %s: %w", cfg.TraceExporter, err)
		}
		logger.Sugar().Infof("using %s trace exporter for service %s", cfg.TraceExporter, svc)

		tp := sdktrace.NewTracerProvider(
			sdktrace.WithBatcher(exp, sdktrace.WithBlocking()),
			sdktrace.WithResource(resource.NewWithAttributes(
				semconv.SchemaURL,
				semconv.ServiceNameKey.String(svc),
			)),
		)
		tracers = append(tracers, tp.Tracer(cfg.Service))
		shutdown = append(shutdown, tp.Shutdown)
	}
	return tracers, func(ctx context.Context) error {
		var errs []error
		for _, f := range shutdown {
			errs = append(errs, f(ctx))
		}
		return errors.Join(errs...)
	}
}

func createOtelExporter(exporterType string) (sdktrace.SpanExporter, error) {
	var exporter sdktrace.SpanExporter
	var err error
	switch exporterType {
	case "jaeger":
		exporter, err = jaeger.New(
			jaeger.WithCollectorEndpoint(),
		)
	case "otlp", "otlp-http":
		client := otlptracehttp.NewClient(
			otlptracehttp.WithInsecure(),
		)
		exporter, err = otlptrace.New(context.Background(), client)
	case "otlp-grpc":
		client := otlptracegrpc.NewClient(
			otlptracegrpc.WithInsecure(),
		)
		exporter, err = otlptrace.New(context.Background(), client)
	case "stdout":
		exporter, err = stdouttrace.New()
	default:
		return nil, fmt.Errorf("unrecognized exporter type %s", exporterType)
	}
	return exporter, err
}
