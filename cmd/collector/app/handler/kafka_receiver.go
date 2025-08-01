// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"context"
	"fmt"

	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/kafkareceiver"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/receiver"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/collector/app/flags"
	"github.com/jaegertracing/jaeger/cmd/collector/app/processor"
	"github.com/jaegertracing/jaeger/internal/tenancy"
)

var (
	kafkaComponentType = component.MustNewType("kafka")
	kafkaID            = component.NewID(kafkaComponentType)
)

// StartKafkaReceiver starts Kafka receiver from OTEL Collector.
func StartKafkaReceiver(
	options *flags.CollectorOptions,
	logger *zap.Logger,
	spanProcessor processor.SpanProcessor,
	tm *tenancy.Manager,
) (receiver.Traces, error) {
	kafkaFactory := kafkareceiver.NewFactory()
	return startKafkaReceiver(
		options,
		logger,
		spanProcessor,
		tm,
		kafkaFactory,
		consumer.NewTraces,
		kafkaFactory.CreateTraces,
	)
}

// Some of OTELCOL constructor functions return errors when passed nil arguments,
// which is a situation we cannot reproduce. To test our own error handling, this
// function allows to mock those constructors.
func startKafkaReceiver(
	options *flags.CollectorOptions,
	logger *zap.Logger,
	spanProcessor processor.SpanProcessor,
	tm *tenancy.Manager,
	// from here: params that can be mocked in tests
	kafkaFactory receiver.Factory,
	newTraces func(consume consumer.ConsumeTracesFunc, options ...consumer.Option) (consumer.Traces, error),
	createTracesReceiver func(ctx context.Context, set receiver.Settings,
		cfg component.Config, nextConsumer consumer.Traces) (receiver.Traces, error),
) (receiver.Traces, error) {
	receiverConfig := kafkaFactory.CreateDefaultConfig().(*kafkareceiver.Config)
	// ClientConfig, ConsumerConfig declarations?
	receiverSettings := receiver.Settings{
		ID: kafkaID,
		TelemetrySettings: component.TelemetrySettings{
			Logger:         logger,
			TracerProvider: nooptrace.NewTracerProvider(),
			MeterProvider:  noopmetric.NewMeterProvider(),
		},
	}

	consumerHelper := &consumerHelper{
		batchConsumer: newBatchConsumer(logger,
			spanProcessor,
			processor.HTTPTransport,
			processor.KafkaSpanFormat,
			tm),
	}

	nextConsumer, err := newTraces(consumerHelper.consume)
	if err != nil {
		return nil, fmt.Errorf("could not create Kafka consumer: %w", err)
	}
	rcvr, err := createTracesReceiver(
		context.Background(),
		receiverSettings,
		receiverConfig,
		nextConsumer,
	)
	if err != nil {
		return nil, fmt.Errorf("could not create Kafka receiver: %w", err)
	}
	if err := rcvr.Start(context.Background(), &otelHost{logger: logger}); err != nil {
		return nil, fmt.Errorf("could not start Kafka receiver: %w", err)
	}
	return rcvr, nil
}
