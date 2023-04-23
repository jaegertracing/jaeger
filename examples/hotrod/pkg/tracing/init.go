// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
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

package tracing

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/opentracing/opentracing-go"
	"go.opentelemetry.io/otel"
	otbridge "go.opentelemetry.io/otel/bridge/opentracing"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.7.0"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/examples/hotrod/pkg/log"
	"github.com/jaegertracing/jaeger/examples/hotrod/pkg/tracing/rpcmetrics"
	"github.com/jaegertracing/jaeger/pkg/metrics"
)

var once sync.Once

// Init initializes OpenTelemetry SDK and uses OTel-OpenTracing Bridge
// to return an OpenTracing-compatible tracer.
func Init(serviceName string, exporterType string, metricsFactory metrics.Factory, logger log.Factory) opentracing.Tracer {
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

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithSpanProcessor(rpcmetricsObserver),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(serviceName),
		)),
	)
	otTracer, _ := otbridge.NewTracerPair(tp.Tracer(""))
	logger.Bg().Debug("created OTEL->OT bridge", zap.String("service-name", serviceName))
	return otTracer
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
		exporter, err = jaeger.New(
			jaeger.WithCollectorEndpoint(),
		)
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
