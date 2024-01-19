// Copyright (c) 2024 The Jaeger Authors.
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

package storagereceiver

import (
	"context"
	"fmt"

	jaeger2otlp "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/jaeger"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/plugin/storage/badger"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

type storageReceiver struct {
	cancelConsumeLoop context.CancelFunc
	config            *Config
	logger            *zap.Logger
	consumedTraces    map[model.TraceID]*consumedTrace
	nextConsumer      consumer.Traces
	spanReader        spanstore.Reader
}

type consumedTrace struct {
	spanIDs map[model.SpanID]struct{}
}

func newReceiver(config *Config, otel component.TelemetrySettings, nextConsumer consumer.Traces) (*storageReceiver, error) {
	f, err := badger.NewFactoryWithConfig(
		config.Badger,
		metrics.NullFactory,
		otel.Logger,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to init storage factory: %w", err)
	}

	spanReader, err := f.CreateSpanReader()
	if err != nil {
		return nil, fmt.Errorf("failed to create span reader: %w", err)
	}

	return &storageReceiver{
		config:         config,
		logger:         otel.Logger,
		consumedTraces: make(map[model.TraceID]*consumedTrace),
		nextConsumer:   nextConsumer,
		spanReader:     spanReader,
	}, nil
}

func (r *storageReceiver) Start(_ context.Context, host component.Host) error {
	ctx, cancel := context.WithCancel(context.Background())
	r.cancelConsumeLoop = cancel

	go func() {
		if err := r.consumeLoop(ctx); err != nil {
			host.ReportFatalError(err)
		}
	}()

	return nil
}

func (r *storageReceiver) consumeLoop(ctx context.Context) error {
	services := []string{"", "customers", "OTLPResourceNoServiceName"}

	for {
		for _, svc := range services {
			if err := r.consumeTraces(ctx, svc); err != nil {
				r.logger.Error("Error from consumer", zap.Error(err))
			}
			if ctx.Err() != nil {
				r.logger.Error("Consumer stopped", zap.Error(ctx.Err()))
				return ctx.Err()
			}
		}
	}
}

func (r *storageReceiver) consumeTraces(ctx context.Context, serviceName string) error {
	traces, err := r.spanReader.FindTraces(ctx, &spanstore.TraceQueryParameters{
		ServiceName: serviceName,
	})
	if err != nil {
		return err
	}

	cnt := 0
	for _, trace := range traces {
		cnt += len(trace.Spans)
		traceID := trace.Spans[0].TraceID
		if _, ok := r.consumedTraces[traceID]; !ok {
			r.consumedTraces[traceID] = &consumedTrace{
				spanIDs: make(map[model.SpanID]struct{}),
			}
		}
		if len(trace.Spans) > len(r.consumedTraces[traceID].spanIDs) {
			r.consumeSpans(ctx, r.consumedTraces[traceID], trace.Spans)
		}
	}

	return nil
}

func (r *storageReceiver) consumeSpans(ctx context.Context, tc *consumedTrace, spans []*model.Span) error {
	for _, span := range spans {
		if _, ok := tc.spanIDs[span.SpanID]; !ok {
			tc.spanIDs[span.SpanID] = struct{}{}
			td, err := jaeger2otlp.ProtoToTraces([]*model.Batch{
				{
					Spans:   []*model.Span{span},
					Process: span.Process,
				},
			})
			if err != nil {
				return err
			}
			r.nextConsumer.ConsumeTraces(ctx, td)
		}
	}

	return nil
}

func (r *storageReceiver) Shutdown(_ context.Context) error {
	r.cancelConsumeLoop()
	return nil
}
