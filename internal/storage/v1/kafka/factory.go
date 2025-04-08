// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package kafka

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/Shopify/sarama"
	"github.com/spf13/viper"
	"go.uber.org/zap"

	model "github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/kafka/producer"
	"github.com/jaegertracing/jaeger/internal/storage/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/dependencystore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
)

var ( // interface comformance checks
	_ storage.Factory      = (*Factory)(nil)
	_ io.Closer            = (*Factory)(nil)
	_ storage.Configurable = (*Factory)(nil)
)

// Factory implements storage.Factory and creates write-only storage components backed by kafka.
type Factory struct {
	options Options

	metricsFactory metrics.Factory
	logger         *zap.Logger

	producer   sarama.AsyncProducer
	marshaller Marshaller
	producer.Builder
}

type spanWriter struct {
	producer       sarama.AsyncProducer
	marshaller     Marshaller
	topic          string
	metricsFactory metrics.Factory
	logger         *zap.Logger
}

// NewFactory creates a new Factory.
func NewFactory() *Factory {
	return &Factory{}
}

// AddFlags implements storage.Configurable
func (f *Factory) AddFlags(flagSet *flag.FlagSet) {
	f.options.AddFlags(flagSet)
}

// InitFromViper implements storage.Configurable
func (f *Factory) InitFromViper(v *viper.Viper, _ *zap.Logger) {
	f.options.InitFromViper(v)
	f.configureFromOptions(f.options)
}

// configureFromOptions initializes factory from options.
func (f *Factory) configureFromOptions(o Options) {
	f.options = o
	f.Builder = &f.options.Config
}

// Initialize implements storage.Factory
func (f *Factory) Initialize(metricsFactory metrics.Factory, logger *zap.Logger) error {
	f.metricsFactory, f.logger = metricsFactory, logger
	logger.Info("Kafka factory",
		zap.Any("producer builder", f.Builder),
		zap.Any("topic", f.options.Topic))
	switch f.options.Encoding {
	case EncodingProto:
		f.marshaller = newProtobufMarshaller()
	case EncodingJSON:
		f.marshaller = newJSONMarshaller()
	default:
		return errors.New("kafka encoding is not one of '" + EncodingJSON + "' or '" + EncodingProto + "'")
	}
	p, err := f.NewProducer(logger)
	if err != nil {
		return err
	}
	f.producer = p
	return nil
}

// WriteSpan implements the spanstore.Writer interface.
func (sw *spanWriter) WriteSpan(ctx context.Context, span *model.Span) error {
	// Marshal the span using the configured marshaller.
	data, err := sw.marshaller.Marshal(span)
	if err != nil {
		return fmt.Errorf("failed to marshal span: %w", err)
	}

	// Create a ProducerMessage for Kafka.
	msg := &sarama.ProducerMessage{
		Topic: sw.topic,
		Value: sarama.ByteEncoder(data),
	}

	// Send the message.
	select {
	case sw.producer.Input() <- msg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (sw *spanWriter) Close() error {
	return sw.producer.Close()
}

// CreateSpanReader implements storage.Factory
func (*Factory) CreateSpanReader() (spanstore.Reader, error) {
	return nil, errors.New("kafka storage is write-only")
}

// CreateSpanWriter implements storage.Factory
func (f *Factory) CreateSpanWriter() (spanstore.Writer, error) {
	if f.producer == nil {
		return nil, fmt.Errorf("kafka producer is not initialized")
	}

	writer := &spanWriter{
		producer:       f.producer,
		marshaller:     f.marshaller,
		topic:          f.options.Topic,
		metricsFactory: f.metricsFactory,
		logger:         f.logger,
	}
	return writer, nil
}

// CreateDependencyReader implements storage.Factory
func (*Factory) CreateDependencyReader() (dependencystore.Reader, error) {
	return nil, errors.New("kafka storage is write-only")
}

var _ io.Closer = (*Factory)(nil)

// Close closes the resources held by the factory
func (f *Factory) Close() error {
	var errs []error
	if f.producer != nil {
		errs = append(errs, f.producer.Close())
	}
	return errors.Join(errs...)
}
