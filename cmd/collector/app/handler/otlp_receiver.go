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
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
)

// A delegation function to assist in tests, because ProtoFromTraces never returns errors despite its API.
var protoFromTraces func(td ptrace.Traces) ([]*model.Batch, error) = otlp2jaeger.ProtoFromTraces

var _ component.Host = (*otelHost)(nil) // API check

type OtelReceiverOptions struct {
	GRPCAddress string
	HTTPAddress string
}

// StartOtelReceiver starts OpenTelemetry OTLP receiver listening on gRPC and HTTP ports.
func StartOtelReceiver(logger *zap.Logger, spanProcessor processor.SpanProcessor) (component.TracesReceiver, error) {
	otlpFactory := otlpreceiver.NewFactory()
	otlpReceiverConfig := otlpFactory.CreateDefaultConfig()
	otlpReceiverSettings := component.ReceiverCreateSettings{
		TelemetrySettings: component.TelemetrySettings{
			Logger:         logger,
			TracerProvider: otel.GetTracerProvider(), // TODO we may always want no-op here, not the global default
		},
	}
	// TODO re-implement the logic of NewGRPCHandler, it's fairly trivial
	jaegerBatchHandler := NewGRPCHandler(logger, spanProcessor)
	nextConsumer, err := consumer.NewTraces(consumer.ConsumeTracesFunc(func(ctx context.Context, ld ptrace.Traces) error {
		batches, err := protoFromTraces(ld)
		if err != nil {
			return err
		}
		for _, batch := range batches {
			// TODO generate metrics
			_, err := jaegerBatchHandler.PostSpans(ctx, &api_v2.PostSpansRequest{
				Batch: *batch,
			})
			if err != nil {
				return err
			}
		}
		return nil
	}))
	if err != nil {
		return nil, fmt.Errorf("could not create the OTLP consumer: %w", err)
	}
	otlpReceiver, err := otlpFactory.CreateTracesReceiver(
		context.Background(),
		otlpReceiverSettings,
		otlpReceiverConfig,
		nextConsumer,
	)
	if err != nil {
		return nil, fmt.Errorf("could not create the OTLP receiver: %w", err)
	}
	err = otlpReceiver.Start(context.Background(), &otelHost{logger: logger})
	if err != nil {
		return nil, fmt.Errorf("could not start the OTLP receiver: %w", err)
	}
	return otlpReceiver, nil
}

// otelHost is a mostly no-op implementation of OTEL component.Host
type otelHost struct {
	logger *zap.Logger
}

func (h *otelHost) ReportFatalError(err error) {
	h.logger.Fatal("OTLP receiver error", zap.Error(err))
}
func (*otelHost) GetFactory(kind component.Kind, componentType config.Type) component.Factory {
	return nil
}
func (*otelHost) GetExtensions() map[config.ComponentID]component.Extension {
	return nil
}
func (*otelHost) GetExporters() map[config.DataType]map[config.ComponentID]component.Exporter {
	return nil
}
