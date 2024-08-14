// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package tracing

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/examples/hotrod/pkg/log"
	"github.com/jaegertracing/jaeger/examples/hotrod/pkg/tracing/rpcmetrics"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/otelsemconv"
)

var once sync.Once

// InitOTEL initializes OpenTelemetry SDK.
func InitOTEL(serviceName string, exporterType string, metricsFactory metrics.Factory, logger log.Factory) trace.TracerProvider {
	once.Do(func() {
		otel.SetTextMapPropagator(
			propagation.NewCompositeTextMapPropagator(
				propagation.TraceContext{},
				propagation.Baggage{},
			))
	})

	exp, err := createOtelExporter(exporterType)
	if err != nil {
		logger.Bg().Fatal("cannot create exporter", zap.String("exporterType", exporterType), zap.Error(err))
	}
	logger.Bg().Debug("using " + exporterType + " trace exporter")

	rpcmetricsObserver := rpcmetrics.NewObserver(metricsFactory, rpcmetrics.DefaultNameNormalizer)

	res, err := resource.New(
		context.Background(),
		resource.WithSchemaURL(otelsemconv.SchemaURL),
		resource.WithAttributes(otelsemconv.ServiceNameKey.String(serviceName)),
		resource.WithTelemetrySDK(),
		resource.WithHost(),
		resource.WithOSType(),
	)
	if err != nil {
		logger.Bg().Fatal("resource creation failed", zap.Error(err))
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp, sdktrace.WithBatchTimeout(1000*time.Millisecond)),
		sdktrace.WithSpanProcessor(rpcmetricsObserver),
		sdktrace.WithResource(res),
	)
	logger.Bg().Debug("Created OTEL tracer", zap.String("service-name", serviceName))
	return tp
}

// withSecure instructs the client to use HTTPS scheme, instead of hotrod's desired default HTTP
func withSecure() bool {
	return strings.HasPrefix(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"), "https://") ||
		strings.ToLower(os.Getenv("OTEL_EXPORTER_OTLP_INSECURE")) == "false"
}

func createOtelExporter(exporterType string) (sdktrace.SpanExporter, error) {
	var exporter sdktrace.SpanExporter
	var err error
	switch exporterType {
	case "jaeger":
		return nil, errors.New("jaeger exporter is no longer supported, please use otlp")
	case "otlp":
		var opts []otlptracehttp.Option
		if !withSecure() {
			opts = []otlptracehttp.Option{otlptracehttp.WithInsecure()}
		}
		exporter, err = otlptrace.New(
			context.Background(),
			otlptracehttp.NewClient(opts...),
		)
	case "stdout":
		exporter, err = stdouttrace.New()
	default:
		return nil, fmt.Errorf("unrecognized exporter type %s", exporterType)
	}
	return exporter, err
}
