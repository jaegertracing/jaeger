// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package kafka

import (
	"bytes"
	"errors"
	"testing"

	"github.com/Shopify/sarama"
	"github.com/Shopify/sarama/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/jaegertracing/jaeger/pkg/config"
	kafkaConfig "github.com/jaegertracing/jaeger/pkg/kafka/producer"
	"github.com/jaegertracing/jaeger/pkg/metrics"
)

type mockProducerBuilder struct {
	kafkaConfig.Configuration
	err error
	t   *testing.T
}

func (m *mockProducerBuilder) NewProducer(*zap.Logger) (sarama.AsyncProducer, error) {
	if m.err == nil {
		return mocks.NewAsyncProducer(m.t, nil), nil
	}
	return nil, m.err
}

func TestKafkaFactory(t *testing.T) {
	f := NewFactory()
	v, command := config.Viperize(f.AddFlags)
	command.ParseFlags([]string{})
	f.InitFromViper(v, zap.NewNop())

	f.Builder = &mockProducerBuilder{
		err: errors.New("made-up error"),
		t:   t,
	}
	require.EqualError(t, f.Initialize(metrics.NullFactory, zap.NewNop()), "made-up error")

	f.Builder = &mockProducerBuilder{t: t}
	require.NoError(t, f.Initialize(metrics.NullFactory, zap.NewNop()))
	assert.IsType(t, &protobufMarshaller{}, f.marshaller)

	_, err := f.CreateSpanWriter()
	require.NoError(t, err)

	_, err = f.CreateSpanReader()
	require.Error(t, err)

	_, err = f.CreateDependencyReader()
	require.Error(t, err)

	require.NoError(t, f.Close())
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
			f.InitFromViper(v, zap.NewNop())

			f.Builder = &mockProducerBuilder{t: t}
			require.NoError(t, f.Initialize(metrics.NullFactory, zap.NewNop()))
			assert.IsType(t, test.marshaller, f.marshaller)
			require.NoError(t, f.Close())
		})
	}
}

func TestKafkaFactoryMarshallerErr(t *testing.T) {
	f := NewFactory()
	v, command := config.Viperize(f.AddFlags)
	command.ParseFlags([]string{"--kafka.producer.encoding=bad-input"})
	f.InitFromViper(v, zap.NewNop())

	f.Builder = &mockProducerBuilder{t: t}
	require.Error(t, f.Initialize(metrics.NullFactory, zap.NewNop()))
}

func TestKafkaFactoryDoesNotLogPassword(t *testing.T) {
	tests := []struct {
		name  string
		flags []string
	}{
		{
			name: "plaintext",
			flags: []string{
				"--kafka.producer.authentication=plaintext",
				"--kafka.producer.plaintext.username=username",
				"--kafka.producer.plaintext.password=SECRET",
				"--kafka.producer.brokers=localhost:9092",
			},
		},
		{
			name: "kerberos",
			flags: []string{
				"--kafka.producer.authentication=kerberos",
				"--kafka.producer.kerberos.username=username",
				"--kafka.producer.kerberos.password=SECRET",
				"--kafka.producer.brokers=localhost:9092",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := NewFactory()
			v, command := config.Viperize(f.AddFlags)
			err := command.ParseFlags(test.flags)
			require.NoError(t, err)

			f.InitFromViper(v, zap.NewNop())

			parsedConfig := f.Builder.(*kafkaConfig.Configuration)
			f.Builder = &mockProducerBuilder{t: t, Configuration: *parsedConfig}
			logbuf := &bytes.Buffer{}
			logger := zap.New(zapcore.NewCore(
				zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
				zapcore.AddSync(logbuf),
				zap.NewAtomicLevel(),
			))
			err = f.Initialize(metrics.NullFactory, logger)
			require.NoError(t, err)
			logger.Sync()

			assert.NotContains(t, logbuf.String(), "SECRET", "log output must not contain password in clear text")
			require.NoError(t, f.Close())
		})
	}
}

func TestConfigureFromOptions(t *testing.T) {
	f := NewFactory()
	o := Options{Topic: "testTopic", Config: kafkaConfig.Configuration{Brokers: []string{"host"}}}
	f.configureFromOptions(o)
	assert.Equal(t, o, f.options)
	assert.Equal(t, &o.Config, f.Builder)
}
