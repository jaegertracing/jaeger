// Copyright (c) 2018 The Jaeger Authors.
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

package consumer

import (
	"io"

	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/ingester/app/consumer/offset"
	"github.com/jaegertracing/jaeger/cmd/ingester/app/processor"
	"github.com/jaegertracing/jaeger/cmd/ingester/app/processor/decorator"
	"github.com/jaegertracing/jaeger/pkg/kafka/consumer"
)

// ProcessorFactoryParams are the parameters of a ProcessorFactory
type ProcessorFactoryParams struct {
	Parallelism          int
	Topic                string
	MaxOutOfOrderOffsets int
	BaseProcessor        processor.SpanProcessor
	SaramaConsumer       consumer.Consumer
	Factory              metrics.Factory
	Logger               *zap.Logger
}

// ProcessorFactory is a factory for creating startedProcessors
type ProcessorFactory struct {
	topic                string
	consumer             consumer.Consumer
	metricsFactory       metrics.Factory
	logger               *zap.Logger
	baseProcessor        processor.SpanProcessor
	parallelism          int
	maxOutOfOrderOffsets int
}

// NewProcessorFactory constructs a new ProcessorFactory
func NewProcessorFactory(params ProcessorFactoryParams) (*ProcessorFactory, error) {
	return &ProcessorFactory{
		topic:                params.Topic,
		consumer:             params.SaramaConsumer,
		metricsFactory:       params.Factory,
		logger:               params.Logger,
		baseProcessor:        params.BaseProcessor,
		parallelism:          params.Parallelism,
		maxOutOfOrderOffsets: params.MaxOutOfOrderOffsets,
	}, nil
}

func (c *ProcessorFactory) new(partition int32, minOffset int64) processor.ParallelSpanProcessor {
	c.logger.Info("Creating new processors", zap.Int32("partition", partition))

	markOffset := func(offset int64) {
		c.consumer.MarkPartitionOffset(c.topic, partition, offset, "")
	}

	om := offset.NewManager(minOffset, c.maxOutOfOrderOffsets, markOffset, partition, c.metricsFactory)

	retryProcessor := decorator.NewRetryingProcessor(c.metricsFactory, c.baseProcessor)
	cp := NewCommittingProcessor(retryProcessor, om)
	spanProcessor := processor.NewDecoratedProcessor(c.metricsFactory, cp)
	pp := processor.NewParallelProcessor(spanProcessor, c.parallelism, c.logger)

	return newStartedProcessor(pp, om)
}

type service interface {
	Start()
	io.Closer
}

type startProcessor interface {
	Start()
	processor.ParallelSpanProcessor
}

type startedProcessor struct {
	services  []service
	processor startProcessor
}

func newStartedProcessor(parallelProcessor startProcessor, services ...service) processor.ParallelSpanProcessor {
	s := &startedProcessor{
		services:  services,
		processor: parallelProcessor,
	}

	for _, service := range services {
		service.Start()
	}

	s.processor.Start()
	return s
}

func (c *startedProcessor) Process(message processor.Message, onError processor.OnError) {
	c.processor.Process(message, onError)
}

func (c *startedProcessor) Close() error {
	c.processor.Close()

	for _, service := range c.services {
		service.Close()
	}
	return nil
}
