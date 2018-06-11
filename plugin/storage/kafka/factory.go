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

package kafka

import (
	"errors"
	"flag"
	"strings"

	"github.com/Shopify/sarama"
	"github.com/spf13/viper"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/kafka/config"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

const (
	configPrefix  = "kafka"
	suffixBrokers = ".brokers"
	suffixTopic   = ".topic"
)

// Factory implements storage.Factory and creates write-only storage components backed by kafka.
type Factory struct {
	metricsFactory metrics.Factory
	logger         *zap.Logger

	config     config.ProducerBuilder
	topic      string
	producer   sarama.AsyncProducer
	marshaller Marshaller
}

// AddFlags implements plugin.Configurable
func (f *Factory) AddFlags(flagSet *flag.FlagSet) {
	flagSet.String(
		configPrefix+suffixBrokers,
		"127.0.0.1:9092",
		"The comma-separated list of kafka brokers. i.e. '127.0.0.1:9092,0.0.0:1234'")
	flagSet.String(
		configPrefix+suffixTopic,
		"jaeger-spans",
		"The name of the kafka topic")
}

// InitFromViper implements plugin.Configurable
func (f *Factory) InitFromViper(v *viper.Viper) {
	f.config = &config.Configuration{
		Brokers: strings.Split(v.GetString(configPrefix+suffixBrokers), ","),
	}
	f.topic = v.GetString(configPrefix + suffixTopic)
}

// NewFactory creates a new Factory.
func NewFactory() *Factory {
	return &Factory{}
}

// Initialize implements storage.Factory
func (f *Factory) Initialize(metricsFactory metrics.Factory, logger *zap.Logger) error {
	f.metricsFactory, f.logger = metricsFactory, logger

	p, err := f.config.NewProducer()
	if err != nil {
		return err
	}
	f.producer = p

	f.marshaller = newThriftMarshaller()

	return nil
}

// CreateSpanReader implements storage.Factory
func (f *Factory) CreateSpanReader() (spanstore.Reader, error) {
	return nil, errors.New("kafka storage is write-only")
}

// CreateSpanWriter implements storage.Factory
func (f *Factory) CreateSpanWriter() (spanstore.Writer, error) {
	return NewSpanWriter(f.producer, f.marshaller, f.topic, f.metricsFactory), nil
}

// CreateDependencyReader implements storage.Factory
func (f *Factory) CreateDependencyReader() (dependencystore.Reader, error) {
	return nil, errors.New("kafka storage is write-only")
}
