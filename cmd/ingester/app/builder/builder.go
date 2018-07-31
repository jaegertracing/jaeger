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

	"github.com/spf13/viper"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/ingester/app/consumer"
	"github.com/jaegertracing/jaeger/cmd/ingester/app/processor"
	kafkaConsumer "github.com/jaegertracing/jaeger/pkg/kafka/consumer"
	"github.com/jaegertracing/jaeger/plugin/storage/kafka"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

const (
	// EncodingJSON indicates spans are encoded as a json byte array
	EncodingJSON = "json"
	// EncodingProto indicates spans are encoded as a protobuf byte array
	EncodingProto = "protobuf"

	// ConfigPrefix is a prefix fro the ingester flags
	ConfigPrefix = "ingester"
	// SuffixBrokers is a suffix for the brokers flag
	SuffixBrokers = ".brokers"
	// SuffixTopic is a suffix for the topic flag
	SuffixTopic = ".topic"
	// SuffixGroupID is a suffix for the group-id flag
	SuffixGroupID = ".group-id"
	// SuffixParallelism is a suffix for the parallelism flag
	SuffixParallelism = ".parallelism"
	// SuffixEncoding is a suffix for the encoding flag
	SuffixEncoding = ".encoding"

	// DefaultBroker is the default kafka broker
	DefaultBroker = "127.0.0.1:9092"
	// DefaultTopic is the default kafka topic
	DefaultTopic = "jaeger-spans"
	// DefaultGroupID is the default consumer Group ID
	DefaultGroupID = "jaeger-ingester"
	// DefaultParallelism is the default parallelism for the span processor
	DefaultParallelism = 1000
	// DefaultEncoding is the default span encoding
	DefaultEncoding = EncodingProto
)

// Builder stores the configuration options for the Ingester
type Builder struct {
	kafkaConsumer.Configuration
	Parallelism int
	Encoding    string
}

// CreateConsumer creates a new span consumer for the ingester
func (b *Builder) CreateConsumer(logger *zap.Logger, metricsFactory metrics.Factory, spanWriter spanstore.Writer) (*consumer.Consumer, error) {
	var unmarshaller kafka.Unmarshaller
	if b.Encoding == EncodingJSON {
		unmarshaller = kafka.NewJSONUnmarshaller()
	} else if b.Encoding == EncodingProto {
		unmarshaller = kafka.NewProtobufUnmarshaller()
	} else {
		return nil, fmt.Errorf(`encoding '%s' not recognised, use one of ("%s" or "%s")`,
			b.Encoding, EncodingProto, EncodingJSON)
	}

	spParams := processor.SpanProcessorParams{
		Writer:       spanWriter,
		Unmarshaller: unmarshaller,
	}
	spanProcessor := processor.NewSpanProcessor(spParams)

	consumerConfig := kafkaConsumer.Configuration{
		Brokers: b.Brokers,
		Topic:   b.Topic,
		GroupID: b.GroupID,
	}
	saramaConsumer, err := consumerConfig.NewConsumer()
	if err != nil {
		return nil, err
	}

	factoryParams := consumer.ProcessorFactoryParams{
		Topic:          b.Topic,
		Parallelism:    b.Parallelism,
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
		InternalConsumer: saramaConsumer,
		ProcessorFactory: *processorFactory,
		Factory:          metricsFactory,
		Logger:           logger,
	}
	return consumer.New(consumerParams)
}

// InitFromViper initializes Builder with properties from viper
func (b *Builder) InitFromViper(v *viper.Viper) {
	b.Brokers = strings.Split(v.GetString(ConfigPrefix+SuffixBrokers), ",")
	b.Topic = v.GetString(ConfigPrefix + SuffixTopic)
	b.GroupID = v.GetString(ConfigPrefix + SuffixGroupID)
	b.Parallelism = v.GetInt(ConfigPrefix + SuffixParallelism)
	b.Encoding = v.GetString(ConfigPrefix + SuffixEncoding)
}
