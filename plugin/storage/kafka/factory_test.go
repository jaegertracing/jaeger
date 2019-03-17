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
	"testing"

	"github.com/Shopify/sarama"
	"github.com/Shopify/sarama/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/config"
	kafkaConfig "github.com/jaegertracing/jaeger/pkg/kafka/producer"
	"github.com/jaegertracing/jaeger/storage"
)

// Checks that Kafka Factory conforms to storage.Factory API
var _ storage.Factory = new(Factory)

type mockProducerBuilder struct {
	kafkaConfig.Configuration
	err error
	t   *testing.T
}

func (m *mockProducerBuilder) NewProducer() (sarama.AsyncProducer, error) {
	if m.err == nil {
		return mocks.NewAsyncProducer(m.t, nil), nil
	}
	return nil, m.err
}

func TestKafkaFactory(t *testing.T) {
	f := NewFactory()
	v, command := config.Viperize(f.AddFlags)
	command.ParseFlags([]string{})
	f.InitFromViper(v)

	f.Builder = &mockProducerBuilder{
		err: errors.New("made-up error"),
		t:   t,
	}
	assert.EqualError(t, f.Initialize(metrics.NullFactory, zap.NewNop()), "made-up error")

	f.Builder = &mockProducerBuilder{t: t}
	assert.NoError(t, f.Initialize(metrics.NullFactory, zap.NewNop()))
	assert.IsType(t, &protobufMarshaller{}, f.marshaller)

	_, err := f.CreateSpanWriter()
	assert.NoError(t, err)

	_, err = f.CreateSpanReader()
	assert.Error(t, err)

	_, err = f.CreateDependencyReader()
	assert.Error(t, err)
}

func TestKafkaFactoryEncoding(t *testing.T) {
	tests := []struct {
		encoding   string
		marshaller Marshaller
	}{
		{encoding: "protobuf", marshaller: new(protobufMarshaller)},
		{encoding: "json", marshaller: new(jsonMarshaller)},
	}
	for _, test := range tests {
		t.Run(test.encoding, func(t *testing.T) {
			f := NewFactory()
			v, command := config.Viperize(f.AddFlags)
			err := command.ParseFlags([]string{"--kafka.producer.encoding=" + test.encoding})
			require.NoError(t, err)
			f.InitFromViper(v)

			f.Builder = &mockProducerBuilder{t: t}
			assert.NoError(t, f.Initialize(metrics.NullFactory, zap.NewNop()))
			assert.IsType(t, test.marshaller, f.marshaller)
		})
	}
}

func TestKafkaFactoryMarshallerErr(t *testing.T) {
	f := NewFactory()
	v, command := config.Viperize(f.AddFlags)
	command.ParseFlags([]string{"--kafka.encoding=bad-input"})
	f.InitFromViper(v)

	f.Builder = &mockProducerBuilder{t: t}
	assert.Error(t, f.Initialize(metrics.NullFactory, zap.NewNop()))
}
