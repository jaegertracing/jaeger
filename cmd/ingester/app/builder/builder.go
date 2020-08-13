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

package builder

import (
	"fmt"
	"strings"

	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/ingester/app"
	"github.com/jaegertracing/jaeger/cmd/ingester/app/consumer"
	"github.com/jaegertracing/jaeger/cmd/ingester/app/processor"
	kafkaConsumer "github.com/jaegertracing/jaeger/pkg/kafka/consumer"
	"github.com/jaegertracing/jaeger/plugin/storage/kafka"
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
	spanProcessor := processor.NewSpanProcessor(spParams)

	consumerConfig := kafkaConsumer.Configuration{
		Brokers:              options.Brokers,
		Topic:                options.Topic,
		GroupID:              options.GroupID,
		ClientID:             options.ClientID,
		ProtocolVersion:      options.ProtocolVersion,
		AuthenticationConfig: options.AuthenticationConfig,
	}
	saramaConsumer, err := consumerConfig.NewConsumer(logger)
	if err != nil {
		return nil, err
	}

	factoryParams := consumer.ProcessorFactoryParams{
		Topic:          options.Topic,
		Parallelism:    options.Parallelism,
		SaramaConsumer: saramaConsumer,
		BaseProcessor:  spanProcessor,
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
