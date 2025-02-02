// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/ingester/app"
	"github.com/jaegertracing/jaeger/cmd/ingester/app/consumer"
	"github.com/jaegertracing/jaeger/cmd/ingester/app/processor"
	"github.com/jaegertracing/jaeger/internal/storage/v1/kafka"
	kafkaConsumer "github.com/jaegertracing/jaeger/pkg/kafka/consumer"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

// CreateConsumer creates a new span consumer for the ingester
func CreateConsumer(logger *zap.Logger, metricsFactory metrics.Factory, spanWriter spanstore.Writer, options app.Options) (*consumer.Consumer, error) {
	var unmarshaller kafka.Unmarshaller
	switch options.Encoding {
	case kafka.EncodingJSON:
		unmarshaller = kafka.NewJSONUnmarshaller()
	case kafka.EncodingProto:
		unmarshaller = kafka.NewProtobufUnmarshaller()
	case kafka.EncodingZipkinThrift:
		unmarshaller = kafka.NewZipkinThriftUnmarshaller()
	default:
		return nil, fmt.Errorf(`encoding '%s' not recognised, use one of ("%s")`,
			options.Encoding, strings.Join(kafka.AllEncodings, "\", \""))
	}

	spParams := processor.SpanProcessorParams{
		Writer:       spanWriter,
		Unmarshaller: unmarshaller,
	}
	proc := processor.NewSpanProcessor(spParams)

	consumerConfig := kafkaConsumer.Configuration{
		Brokers:              options.Brokers,
		Topic:                options.Topic,
		InitialOffset:        options.InitialOffset,
		GroupID:              options.GroupID,
		ClientID:             options.ClientID,
		ProtocolVersion:      options.ProtocolVersion,
		AuthenticationConfig: options.AuthenticationConfig,
		RackID:               options.RackID,
		FetchMaxMessageBytes: options.FetchMaxMessageBytes,
	}
	saramaConsumer, err := consumerConfig.NewConsumer(logger)
	if err != nil {
		return nil, err
	}

	factoryParams := consumer.ProcessorFactoryParams{
		Parallelism:    options.Parallelism,
		SaramaConsumer: saramaConsumer,
		BaseProcessor:  proc,
		Logger:         logger,
		Factory:        metricsFactory,
	}
	processorFactory, err := consumer.NewProcessorFactory(factoryParams)
	if err != nil {
		return nil, err
	}

	consumerParams := consumer.Params{
		InternalConsumer:      saramaConsumer,
		ProcessorFactory:      *processorFactory,
		MetricsFactory:        metricsFactory,
		Logger:                logger,
		DeadlockCheckInterval: options.DeadlockInterval,
	}
	return consumer.New(consumerParams)
}
