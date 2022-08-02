// Copyright (c) 2022 The Jaeger Authors.
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

package handler

import (
	"context"
	"fmt"

	otlp2jaeger "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/jaeger"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/config/configtls"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/receiver/otlpreceiver"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/collector/app/flags"
	"github.com/jaegertracing/jaeger/cmd/collector/app/processor"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
)

var _ component.Host = (*otelHost)(nil) // API check

// StartOTLPReceiver starts OpenTelemetry OTLP receiver listening on gRPC and HTTP ports.
func StartOTLPReceiver(options *flags.CollectorOptions, logger *zap.Logger, spanProcessor processor.SpanProcessor, tm *tenancy.Manager) (component.TracesReceiver, error) {
	otlpFactory := otlpreceiver.NewFactory()
	return startOTLPReceiver(
		options,
		logger,
		spanProcessor,
		tm,
		otlpFactory,
		consumer.NewTraces,
		otlpFactory.CreateTracesReceiver,
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
	otlpFactory component.ReceiverFactory,
	newTraces func(consume consumer.ConsumeTracesFunc, options ...consumer.Option) (consumer.Traces, error),
	createTracesReceiver func(ctx context.Context, set component.ReceiverCreateSettings,
		cfg config.Receiver, nextConsumer consumer.Traces) (component.TracesReceiver, error),
) (component.TracesReceiver, error) {
	otlpReceiverConfig := otlpFactory.CreateDefaultConfig().(*otlpreceiver.Config)
	applyGRPCSettings(otlpReceiverConfig.GRPC, &options.OTLP.GRPC)
	applyHTTPSettings(otlpReceiverConfig.HTTP, &options.OTLP.HTTP)
	otlpReceiverSettings := component.ReceiverCreateSettings{
		TelemetrySettings: component.TelemetrySettings{
			Logger:         logger,
			TracerProvider: otel.GetTracerProvider(), // TODO we may always want no-op here, not the global default
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
	if err := otlpReceiver.Start(context.Background(), &otelHost{logger: logger}); err != nil {
		return nil, fmt.Errorf("could not start the OTLP receiver: %w", err)
	}
	return otlpReceiver, nil
}

func applyGRPCSettings(cfg *configgrpc.GRPCServerSettings, opts *flags.GRPCOptions) {
	if opts.HostPort != "" {
		cfg.NetAddr.Endpoint = opts.HostPort
	}
	if opts.TLS.Enabled {
		cfg.TLSSetting = applyTLSSettings(&opts.TLS)
	}
	if opts.MaxReceiveMessageLength > 0 {
		cfg.MaxRecvMsgSizeMiB = uint64(opts.MaxReceiveMessageLength / (1024 * 1024))
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

func applyHTTPSettings(cfg *confighttp.HTTPServerSettings, opts *flags.HTTPOptions) {
	if opts.HostPort != "" {
		cfg.Endpoint = opts.HostPort
	}
	if opts.TLS.Enabled {
		cfg.TLSSetting = applyTLSSettings(&opts.TLS)
	}
}

func applyTLSSettings(opts *tlscfg.Options) *configtls.TLSServerSetting {
	return &configtls.TLSServerSetting{
		TLSSetting: configtls.TLSSetting{
			CAFile:     opts.CAPath,
			CertFile:   opts.CertPath,
			KeyFile:    opts.KeyPath,
			MinVersion: opts.MinVersion,
			MaxVersion: opts.MaxVersion,
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

// otelHost is a mostly no-op implementation of OTEL component.Host
type otelHost struct {
	logger *zap.Logger
}

func (h *otelHost) ReportFatalError(err error) {
	h.logger.Fatal("OTLP receiver error", zap.Error(err))
}

func (*otelHost) GetFactory(_ component.Kind, _ config.Type) component.Factory {
	return nil
}

func (*otelHost) GetExtensions() map[config.ComponentID]component.Extension {
	return nil
}

func (*otelHost) GetExporters() map[config.DataType]map[config.ComponentID]component.Exporter {
	return nil
}
