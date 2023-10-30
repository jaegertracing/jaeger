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
	"testing"

	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	pulsarConfig "github.com/jaegertracing/jaeger/pkg/pulsar/producer"
	"github.com/jaegertracing/jaeger/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// Checks that Kafka Factory conforms to storage.Factory API
var _ storage.Factory = new(Factory)

func TestPulsarFactory(t *testing.T) {
	f := NewFactory()
	v, command := config.Viperize(f.AddFlags)
	command.ParseFlags([]string{})
	f.InitFromViper(v, zap.NewNop())

	assert.Nil(t, f.Initialize(metrics.NullFactory, zap.NewNop()))

	_, err := f.CreateSpanWriter()
	assert.NoError(t, err)

	_, err = f.CreateSpanReader()
	assert.Error(t, err)

	_, err = f.CreateDependencyReader()
	assert.Error(t, err)

	assert.NoError(t, f.Close())
}

func TestPulsarFactoryEncoding(t *testing.T) {
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
			err := command.ParseFlags([]string{"--pulsar.producer.encoding=" + test.encoding})
			require.NoError(t, err)
			f.InitFromViper(v, zap.NewNop())

			assert.NoError(t, f.Initialize(metrics.NullFactory, zap.NewNop()))
			assert.IsType(t, test.marshaller, f.marshaller)
		})
	}
}

func TestPulsarFactoryMarshallerErr(t *testing.T) {
	f := NewFactory()
	v, command := config.Viperize(f.AddFlags)
	command.ParseFlags([]string{"--pulsar.producer.encoding=bad-input"})
	f.InitFromViper(v, zap.NewNop())
	assert.Error(t, f.Initialize(metrics.NullFactory, zap.NewNop()))
}

func TestInitFromOptions(t *testing.T) {
	f := NewFactory()
	o := Options{Config: pulsarConfig.Configuration{Topic: "test-topic"}}
	f.InitFromOptions(o)
	assert.Equal(t, o, f.options)
	assert.Equal(t, &o.Config, f.Builder)
}
