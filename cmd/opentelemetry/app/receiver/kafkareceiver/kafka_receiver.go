// Copyright (c) 2020 The Jaeger Authors.
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

package kafkareceiver

import (
	"context"

	"github.com/uber/jaeger-lib/metrics"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/obsreport"
	jaegertranslator "go.opentelemetry.io/collector/translator/trace/jaeger"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/ingester/app/builder"
	ingester "github.com/jaegertracing/jaeger/cmd/ingester/app/consumer"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

var (
	_ spanstore.Writer   = (*writer)(nil)
	_ component.Receiver = (*kafkaReceiver)(nil)
)

type kafkaReceiver struct {
	logger   *zap.Logger
	consumer *ingester.Consumer
}

type writer struct {
	receiver     string
	nextConsumer consumer.TraceConsumer
}

func new(
	config *Config,
	nextConsumer consumer.TraceConsumer,
	params component.ReceiverCreateParams,
) (component.TraceReceiver, error) {
	w := &writer{receiver: config.Name(), nextConsumer: nextConsumer}
	consumer, err := builder.CreateConsumer(
		params.Logger,
		metrics.NullFactory,
		w,
		config.Options)
	if err != nil {
		return nil, err
	}
	return &kafkaReceiver{
		consumer: consumer,
		logger:   params.Logger,
	}, nil
}

// Start starts the receiver.
func (r kafkaReceiver) Start(_ context.Context, _ component.Host) error {
	r.consumer.Start()
	return nil
}

// Shutdown shutdowns the receiver.
func (r kafkaReceiver) Shutdown(_ context.Context) error {
	return r.consumer.Close()
}

// WriteSpan writes a span to the next consumer.
func (w writer) WriteSpan(ctx context.Context, span *model.Span) error {
	batch := model.Batch{
		Spans:   []*model.Span{span},
		Process: span.Process,
	}
	traces := jaegertranslator.ProtoBatchToInternalTraces(batch)
	return w.nextConsumer.ConsumeTraces(w.addContextMetrics(ctx), traces)
}

// addContextMetrics decorates the context with labels used in metrics later.
func (w writer) addContextMetrics(ctx context.Context) context.Context {
	// TODO too many mallocs here, should be a cheaper way
	ctx = obsreport.ReceiverContext(ctx, w.receiver, "kafka", "kafka")
	ctx = obsreport.StartTraceDataReceiveOp(ctx, TypeStr, "kafka")
	return ctx
}
