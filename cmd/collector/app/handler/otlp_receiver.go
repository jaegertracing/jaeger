// Copyright (c) 2022 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"context"
	"fmt"

	otlp2jaeger "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/jaeger"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componentstatus"
	"go.opentelemetry.io/collector/config/confignet"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pipeline"
	"go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/collector/receiver/otlpreceiver"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/collector/app/flags"
	"github.com/jaegertracing/jaeger/cmd/collector/app/processor"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
)

var _ component.Host = (*otelHost)(nil) // API check

// StartOTLPReceiver starts OpenTelemetry OTLP receiver listening on gRPC and HTTP ports.
func StartOTLPReceiver(options *flags.CollectorOptions, logger *zap.Logger, spanProcessor processor.SpanProcessor, tm *tenancy.Manager) (receiver.Traces, error) {
	otlpFactory := otlpreceiver.NewFactory()
	return startOTLPReceiver(
		options,
		logger,
		spanProcessor,
		tm,
		otlpFactory,
		consumer.NewTraces,
		otlpFactory.CreateTraces,
	)
}

// Some of OTELCOL constructor functions return errors when passed nil arguments,
// which is a situation we cannot reproduce. To test our own error handling, this
// function allows to mock those constructors.
func startOTLPReceiver(
	options *flags.CollectorOptions,
	logger *zap.Logger,
	spanProcessor processor.SpanProcessor,
	tm *tenancy.Manager,
	// from here: params that can be mocked in tests
	otlpFactory receiver.Factory,
	newTraces func(consume consumer.ConsumeTracesFunc, options ...consumer.Option) (consumer.Traces, error),
	createTracesReceiver func(ctx context.Context, set receiver.Settings,
		cfg component.Config, nextConsumer consumer.Traces) (receiver.Traces, error),
) (receiver.Traces, error) {
	otlpReceiverConfig := otlpFactory.CreateDefaultConfig().(*otlpreceiver.Config)
	otlpReceiverConfig.GRPC = &options.OTLP.GRPC
	otlpReceiverConfig.GRPC.NetAddr.Transport = confignet.TransportTypeTCP
	otlpReceiverConfig.HTTP.ServerConfig = &options.OTLP.HTTP
	statusReporter := func(ev *componentstatus.Event) {
		// TODO this could be wired into changing healthcheck.HealthCheck
		logger.Info("OTLP receiver status change", zap.Stringer("status", ev.Status()))
	}
	otlpReceiverSettings := receiver.Settings{
		TelemetrySettings: component.TelemetrySettings{
			Logger:         logger,
			TracerProvider: nooptrace.NewTracerProvider(),
			MeterProvider:  noopmetric.NewMeterProvider(),
		},
	}

	otlpConsumer := newConsumerDelegate(logger, spanProcessor, tm)
	// the following two constructors never return errors given non-nil arguments, so we ignore errors
	nextConsumer, err := newTraces(otlpConsumer.consume)
	if err != nil {
		return nil, fmt.Errorf("could not create the OTLP consumer: %w", err)
	}
	otlpReceiver, err := createTracesReceiver(
		context.Background(),
		otlpReceiverSettings,
		otlpReceiverConfig,
		nextConsumer,
	)
	if err != nil {
		return nil, fmt.Errorf("could not create the OTLP receiver: %w", err)
	}
	if err := otlpReceiver.Start(context.Background(), &otelHost{logger: logger, reportFunc: statusReporter}); err != nil {
		return nil, fmt.Errorf("could not start the OTLP receiver: %w", err)
	}
	return otlpReceiver, nil
}

func newConsumerDelegate(logger *zap.Logger, spanProcessor processor.SpanProcessor, tm *tenancy.Manager) *consumerDelegate {
	return &consumerDelegate{
		batchConsumer: newBatchConsumer(logger,
			spanProcessor,
			processor.UnknownTransport, // could be gRPC or HTTP
			processor.OTLPSpanFormat,
			tm),
	}
}

type consumerDelegate struct {
	batchConsumer batchConsumer
}

func (c *consumerDelegate) consume(ctx context.Context, td ptrace.Traces) error {
	batches := otlp2jaeger.ProtoFromTraces(td)
	for _, batch := range batches {
		err := c.batchConsumer.consume(ctx, batch)
		if err != nil {
			return err
		}
	}
	return nil
}

var _ componentstatus.Reporter = (*otelHost)(nil)

// otelHost is a mostly no-op implementation of OTEL component.Host
type otelHost struct {
	logger *zap.Logger

	reportFunc func(event *componentstatus.Event)
}

func (h *otelHost) ReportFatalError(err error) {
	h.logger.Fatal("OTLP receiver error", zap.Error(err))
}

func (*otelHost) GetFactory(_ component.Kind, _ pipeline.Signal) component.Factory {
	return nil
}

func (*otelHost) GetExtensions() map[component.ID]extension.Extension {
	return nil
}

func (*otelHost) GetExporters() map[pipeline.Signal]map[component.ID]component.Component {
	return nil
}

func (h *otelHost) Report(event *componentstatus.Event) {
	h.reportFunc(event)
}
