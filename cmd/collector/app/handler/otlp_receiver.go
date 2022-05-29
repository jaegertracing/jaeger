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
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/receiver/otlpreceiver"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/collector/app/processor"
	"github.com/jaegertracing/jaeger/model"
)

var _ component.Host = (*otelHost)(nil) // API check

// OtelReceiverOptions allows configuration of the receiver.
type OtelReceiverOptions struct {
	GRPCHostPort string
	HTTPHostPort string
}

// StartOtelReceiver starts OpenTelemetry OTLP receiver listening on gRPC and HTTP ports.
func StartOtelReceiver(options OtelReceiverOptions, logger *zap.Logger, spanProcessor processor.SpanProcessor) (component.TracesReceiver, error) {
	otlpFactory := otlpreceiver.NewFactory()
	otlpReceiverConfig := otlpFactory.CreateDefaultConfig().(*otlpreceiver.Config)
	if options.GRPCHostPort != "" {
		otlpReceiverConfig.GRPC.NetAddr.Endpoint = options.GRPCHostPort
	}
	if options.HTTPHostPort != "" {
		otlpReceiverConfig.HTTP.Endpoint = options.HTTPHostPort
	}
	otlpReceiverSettings := component.ReceiverCreateSettings{
		TelemetrySettings: component.TelemetrySettings{
			Logger:         logger,
			TracerProvider: otel.GetTracerProvider(), // TODO we may always want no-op here, not the global default
		},
	}

	otlpConsumer := newConsumerDelegate(logger, spanProcessor)
	// the following two constructors never return errors given non-nil arguments, so we ignore errors
	nextConsumer, _ := consumer.NewTraces(otlpConsumer.consume)
	otlpReceiver, _ := otlpFactory.CreateTracesReceiver(
		context.Background(),
		otlpReceiverSettings,
		otlpReceiverConfig,
		nextConsumer,
	)
	if err := otlpReceiver.Start(context.Background(), &otelHost{logger: logger}); err != nil {
		return nil, fmt.Errorf("could not start the OTLP receiver: %w", err)
	}
	return otlpReceiver, nil
}

func newConsumerDelegate(logger *zap.Logger, spanProcessor processor.SpanProcessor) *consumerDelegate {
	return &consumerDelegate{
		batchConsumer: batchConsumer{
			logger:        logger,
			spanProcessor: spanProcessor,
			spanOptions: processor.SpansOptions{
				SpanFormat:       processor.OTLPSpanFormat,
				InboundTransport: processor.UnknownTransport, // could be gRPC or HTTP
			},
		},
		protoFromTraces: otlp2jaeger.ProtoFromTraces,
	}
}

type consumerDelegate struct {
	batchConsumer   batchConsumer
	protoFromTraces func(td ptrace.Traces) ([]*model.Batch, error)
}

func (c *consumerDelegate) consume(_ context.Context, td ptrace.Traces) error {
	batches, err := c.protoFromTraces(td)
	if err != nil {
		return err
	}
	for _, batch := range batches {
		err := c.batchConsumer.consume(batch)
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
