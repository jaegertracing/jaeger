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

package pulsar

import (
	"errors"
	"flag"
	"io"

	"github.com/apache/pulsar-client-go/pulsar"
	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/pulsar/producer"
	"github.com/jaegertracing/jaeger/plugin"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

var (
	_ io.Closer           = (*Factory)(nil)
	_ plugin.Configurable = (*Factory)(nil)
)

// Factory implements storage.Factory and creates write-only storage components backed by pulsar.
type Factory struct {
	options Options

	metricsFactory metrics.Factory
	logger         *zap.Logger

	producer   pulsar.Producer
	marshaller Marshaller
	producer.Builder
}

// NewFactory creates a new Factory.
func NewFactory() *Factory {
	return &Factory{}
}

// AddFlags implements plugin.Configurable
func (f *Factory) AddFlags(flagSet *flag.FlagSet) {
	f.options.AddFlags(flagSet)
}

// InitFromViper implements plugin.Configurable
func (f *Factory) InitFromViper(v *viper.Viper, logger *zap.Logger) {
	f.options.InitFromViper(v)
	f.Builder = &f.options.Config
}

// InitFromOptions initializes factory from options.
func (f *Factory) InitFromOptions(o Options) {
	f.options = o
	f.Builder = &f.options.Config
}

// Initialize implements storage.Factory
func (f *Factory) Initialize(metricsFactory metrics.Factory, logger *zap.Logger) error {
	f.metricsFactory, f.logger = metricsFactory, logger
	logger.Info("Pulsar factory",
		zap.Any("producer builder", f.Builder),
		zap.Any("topic", f.options.Config.Topic))
	p, err := f.NewProducer(f.options.Config.Topic, logger)
	if err != nil {
		return err
	}
	f.producer = p
	switch f.options.Encoding {
	case EncodingProto:
		f.marshaller = newProtobufMarshaller()
	case EncodingJSON:
		f.marshaller = newJSONMarshaller()
	default:
		return errors.New("pulsar encoding is not one of '" + EncodingJSON + "' or '" + EncodingProto + "'")
	}
	return nil
}

// CreateSpanReader implements storage.Factory
func (f *Factory) CreateSpanReader() (spanstore.Reader, error) {
	return nil, errors.New("pulsar storage is write-only")
}

// CreateSpanWriter implements storage.Factory
func (f *Factory) CreateSpanWriter() (spanstore.Writer, error) {
	return NewSpanWriter(f.producer, f.marshaller, f.metricsFactory, f.logger), nil
}

// CreateDependencyReader implements storage.Factory
func (f *Factory) CreateDependencyReader() (dependencystore.Reader, error) {
	return nil, errors.New("pulsar storage is write-only")
}

var _ io.Closer = (*Factory)(nil)

// Close closes the resources held by the factory
func (f *Factory) Close() error {
	return nil
}
