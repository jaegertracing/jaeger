// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package consumer

import (
	"io"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/ingester/app/consumer/offset"
	"github.com/jaegertracing/jaeger/cmd/ingester/app/processor"
	"github.com/jaegertracing/jaeger/cmd/ingester/app/processor/decorator"
	"github.com/jaegertracing/jaeger/pkg/kafka/consumer"
	"github.com/jaegertracing/jaeger/pkg/metrics"
)

// ProcessorFactoryParams are the parameters of a ProcessorFactory
type ProcessorFactoryParams struct {
	Parallelism    int
	BaseProcessor  processor.SpanProcessor
	SaramaConsumer consumer.Consumer
	Factory        metrics.Factory
	Logger         *zap.Logger
	RetryOptions   []decorator.RetryOption
}

// ProcessorFactory is a factory for creating startedProcessors
type ProcessorFactory struct {
	consumer       consumer.Consumer
	metricsFactory metrics.Factory
	logger         *zap.Logger
	baseProcessor  processor.SpanProcessor
	parallelism    int
	retryOptions   []decorator.RetryOption
}

// NewProcessorFactory constructs a new ProcessorFactory
func NewProcessorFactory(params ProcessorFactoryParams) (*ProcessorFactory, error) {
	return &ProcessorFactory{
		consumer:       params.SaramaConsumer,
		metricsFactory: params.Factory,
		logger:         params.Logger,
		baseProcessor:  params.BaseProcessor,
		parallelism:    params.Parallelism,
		retryOptions:   params.RetryOptions,
	}, nil
}

func (c *ProcessorFactory) new(topic string, partition int32, minOffset int64) processor.SpanProcessor {
	c.logger.Info("Creating new processors", zap.Int32("partition", partition))

	markOffset := func(offsetVal int64) {
		c.consumer.MarkPartitionOffset(topic, partition, offsetVal, "")
	}

	om := offset.NewManager(minOffset, markOffset, topic, partition, c.metricsFactory)

	retryProcessor := decorator.NewRetryingProcessor(c.metricsFactory, c.baseProcessor, c.retryOptions...)
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
	processor.SpanProcessor
}

type startedProcessor struct {
	services  []service
	processor startProcessor
}

func newStartedProcessor(parallelProcessor startProcessor, services ...service) processor.SpanProcessor {
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

func (c *startedProcessor) Process(message processor.Message) error {
	return c.processor.Process(message)
}

func (c *startedProcessor) Close() error {
	c.processor.Close()

	for _, service := range c.services {
		service.Close()
	}
	return nil
}
