// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"context"
	"fmt"

	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/zipkinreceiver"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configtelemetry"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/otel/metric"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/collector/app/flags"
	"github.com/jaegertracing/jaeger/cmd/collector/app/processor"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
)

// StartZipkinReceiver starts Zipkin receiver from OTEL Collector.
func StartZipkinReceiver(
	options *flags.CollectorOptions,
	logger *zap.Logger,
	spanProcessor processor.SpanProcessor,
	tm *tenancy.Manager,
) (receiver.Traces, error) {
	zipkinFactory := zipkinreceiver.NewFactory()
	return startZipkinReceiver(
		options,
		logger,
		spanProcessor,
		tm,
		zipkinFactory,
		consumer.NewTraces,
		zipkinFactory.CreateTraces,
	)
}

// Some of OTELCOL constructor functions return errors when passed nil arguments,
// which is a situation we cannot reproduce. To test our own error handling, this
// function allows to mock those constructors.
func startZipkinReceiver(
	options *flags.CollectorOptions,
	logger *zap.Logger,
	spanProcessor processor.SpanProcessor,
	tm *tenancy.Manager,
	// from here: params that can be mocked in tests
	zipkinFactory receiver.Factory,
	newTraces func(consume consumer.ConsumeTracesFunc, options ...consumer.Option) (consumer.Traces, error),
	createTracesReceiver func(ctx context.Context, set receiver.Settings,
		cfg component.Config, nextConsumer consumer.Traces) (receiver.Traces, error),
) (receiver.Traces, error) {
	receiverConfig := zipkinFactory.CreateDefaultConfig().(*zipkinreceiver.Config)
	applyHTTPSettings(&receiverConfig.ServerConfig, &flags.HTTPOptions{
		HostPort: options.Zipkin.HTTPHostPort,
		TLS:      options.Zipkin.TLS,
		CORS:     options.HTTP.CORS,
		// TODO keepAlive not supported?
	})
	receiverSettings := receiver.Settings{
		TelemetrySettings: component.TelemetrySettings{
			Logger:         logger,
			TracerProvider: nooptrace.NewTracerProvider(),
			// TODO wire this with jaegerlib metrics?
			LeveledMeterProvider: func(_ configtelemetry.Level) metric.MeterProvider {
				return noopmetric.NewMeterProvider()
			},
			MeterProvider: noopmetric.NewMeterProvider(),
		},
	}

	consumerAdapter := newConsumerDelegate(logger, spanProcessor, tm)
	// reset Zipkin spanFormat
	consumerAdapter.batchConsumer.spanOptions.SpanFormat = processor.ZipkinSpanFormat

	nextConsumer, err := newTraces(consumerAdapter.consume)
	if err != nil {
		return nil, fmt.Errorf("could not create Zipkin consumer: %w", err)
	}
	rcvr, err := createTracesReceiver(
		context.Background(),
		receiverSettings,
		receiverConfig,
		nextConsumer,
	)
	if err != nil {
		return nil, fmt.Errorf("could not create Zipkin receiver: %w", err)
	}
	if err := rcvr.Start(context.Background(), &otelHost{logger: logger}); err != nil {
		return nil, fmt.Errorf("could not start Zipkin receiver: %w", err)
	}
	return rcvr, nil
}
