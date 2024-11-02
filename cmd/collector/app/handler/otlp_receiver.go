// Copyright (c) 2022 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"context"
	"fmt"

	otlp2jaeger "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/jaeger"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componentstatus"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/config/configtelemetry"
	"go.opentelemetry.io/collector/config/configtls"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pipeline"
	"go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/collector/receiver/otlpreceiver"
	"go.opentelemetry.io/otel/metric"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/collector/app/flags"
	"github.com/jaegertracing/jaeger/cmd/collector/app/processor"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
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
	applyGRPCSettings(otlpReceiverConfig.GRPC, &options.OTLP.GRPC)
	applyHTTPSettings(otlpReceiverConfig.HTTP.ServerConfig, &options.OTLP.HTTP)
	statusReporter := func(ev *componentstatus.Event) {
		// TODO this could be wired into changing healthcheck.HealthCheck
		logger.Info("OTLP receiver status change", zap.Stringer("status", ev.Status()))
	}
	otlpReceiverSettings := receiver.Settings{
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

func applyGRPCSettings(cfg *configgrpc.ServerConfig, opts *flags.GRPCOptions) {
	if opts.HostPort != "" {
		cfg.NetAddr.Endpoint = opts.HostPort
	}
	if opts.TLS.Enabled {
		cfg.TLSSetting = applyTLSSettings(&opts.TLS)
	}
	if opts.MaxReceiveMessageLength > 0 {
		cfg.MaxRecvMsgSizeMiB = int(opts.MaxReceiveMessageLength / (1024 * 1024))
	}
	if opts.MaxConnectionAge != 0 || opts.MaxConnectionAgeGrace != 0 {
		cfg.Keepalive = &configgrpc.KeepaliveServerConfig{
			ServerParameters: &configgrpc.KeepaliveServerParameters{
				MaxConnectionAge:      opts.MaxConnectionAge,
				MaxConnectionAgeGrace: opts.MaxConnectionAgeGrace,
			},
		}
	}
}

func applyHTTPSettings(cfg *confighttp.ServerConfig, opts *flags.HTTPOptions) {
	if opts.HostPort != "" {
		cfg.Endpoint = opts.HostPort
	}
	if opts.TLS.Enabled {
		cfg.TLSSetting = applyTLSSettings(&opts.TLS)
	}

	cfg.CORS = &confighttp.CORSConfig{
		AllowedOrigins: opts.CORS.AllowedOrigins,
		AllowedHeaders: opts.CORS.AllowedHeaders,
	}
}

func applyTLSSettings(opts *tlscfg.Options) *configtls.ServerConfig {
	return &configtls.ServerConfig{
		Config: configtls.Config{
			CAFile:         opts.CAPath,
			CertFile:       opts.CertPath,
			KeyFile:        opts.KeyPath,
			MinVersion:     opts.MinVersion,
			MaxVersion:     opts.MaxVersion,
			ReloadInterval: opts.ReloadInterval,
		},
		ClientCAFile: opts.ClientCAPath,
	}
}

func newConsumerDelegate(logger *zap.Logger, spanProcessor processor.SpanProcessor, tm *tenancy.Manager) *consumerDelegate {
	return &consumerDelegate{
		batchConsumer: newBatchConsumer(logger,
			spanProcessor,
			processor.UnknownTransport, // could be gRPC or HTTP
			processor.OTLPSpanFormat,
			tm),
		protoFromTraces: otlp2jaeger.ProtoFromTraces,
	}
}

type consumerDelegate struct {
	batchConsumer   batchConsumer
	protoFromTraces func(td ptrace.Traces) ([]*model.Batch, error)
}

func (c *consumerDelegate) consume(ctx context.Context, td ptrace.Traces) error {
	batches, err := c.protoFromTraces(td)
	if err != nil {
		return err
	}
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
