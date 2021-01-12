// Copyright (c) 2020 The Jaeger Authors.
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

package kafkareceiver

import (
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/configcheck"
	"go.opentelemetry.io/collector/config/configtest"
	otelKafkaReceiver "go.opentelemetry.io/collector/receiver/kafkareceiver"

	"github.com/jaegertracing/jaeger/cmd/flags"
	jConfig "github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/plugin/storage/kafka"
)

func TestDefaultConfig(t *testing.T) {
	v, c := jConfig.Viperize(AddFlags)
	err := c.ParseFlags([]string{""})
	require.NoError(t, err)

	factory := &Factory{
		Wrapped: otelKafkaReceiver.NewFactory(),
		Viper:   v,
	}
	defaultCfg := factory.CreateDefaultConfig().(*otelKafkaReceiver.Config)

	assert.NoError(t, configcheck.ValidateConfig(defaultCfg))
	assert.Equal(t, "jaeger-spans", defaultCfg.Topic)
	assert.Equal(t, "jaeger_proto", defaultCfg.Encoding)
	assert.Equal(t, []string{"127.0.0.1:9092"}, defaultCfg.Brokers)
	assert.Equal(t, "jaeger-ingester", defaultCfg.ClientID)
	assert.Equal(t, "jaeger-ingester", defaultCfg.GroupID)
	assert.Equal(t, "0.10.2.0", defaultCfg.ProtocolVersion)
	assert.Nil(t, defaultCfg.Authentication.Kerberos)
	assert.Nil(t, defaultCfg.Authentication.TLS)
	assert.Nil(t, defaultCfg.Authentication.PlainText)
}

func TestLoadConfigAndFlags(t *testing.T) {
	v, c := jConfig.Viperize(AddFlags, flags.AddConfigFileFlag)
	err := c.ParseFlags([]string{"--config-file=./testdata/jaeger-config.yaml", "--kafka.consumer.topic=jaeger-test", "--kafka.consumer.brokers=host1,host2", "--kafka.consumer.group-id=from-flag", "--kafka.consumer.protocol-version=1.1", "--kafka.consumer.kerberos.realm=from-flag"})
	require.NoError(t, err)

	err = flags.TryLoadConfigFile(v)
	require.NoError(t, err)

	factory := &Factory{
		Wrapped: otelKafkaReceiver.NewFactory(),
		Viper:   v,
	}

	factories, err := componenttest.ExampleComponents()
	require.NoError(t, err)
	factories.Receivers[TypeStr] = factory
	cfg, err := configtest.LoadConfigFile(t, path.Join(".", "testdata", "config.yaml"), factories)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	kafkaCfg := cfg.Receivers[TypeStr].(*otelKafkaReceiver.Config)
	require.NotNil(t, kafkaCfg)

	assert.Equal(t, TypeStr, kafkaCfg.Name())
	assert.Equal(t, "jaeger-prod", kafkaCfg.Topic)
	assert.Equal(t, "emojis", kafkaCfg.Encoding)
	assert.Equal(t, "1.1", kafkaCfg.ProtocolVersion)
	assert.Equal(t, []string{"foo", "bar"}, kafkaCfg.Brokers)
	assert.Equal(t, "user", kafkaCfg.Authentication.PlainText.Username)
	assert.Equal(t, "123", kafkaCfg.Authentication.PlainText.Password)
	assert.Equal(t, "ca.crt", kafkaCfg.Authentication.TLS.CAFile)
	assert.Equal(t, "key.crt", kafkaCfg.Authentication.TLS.KeyFile)
	assert.Equal(t, true, kafkaCfg.Authentication.TLS.Insecure)
	assert.Equal(t, "from-flag", kafkaCfg.Authentication.Kerberos.Realm)
	assert.Equal(t, "/etc/foo", kafkaCfg.Authentication.Kerberos.ConfigPath)
	assert.Equal(t, "from-jaeger-config", kafkaCfg.Authentication.Kerberos.Username)
}

func TestMustOtelEncodingForJaegerEncoding(t *testing.T) {
	tests := []struct {
		in           string
		expected     string
		expectsPanic bool
	}{
		{
			in:       kafka.EncodingProto,
			expected: "jaeger_proto",
		},
		{
			in:       kafka.EncodingJSON,
			expected: "jaeger_json",
		},
		{
			in:       encodingOTLPProto,
			expected: "otlp_proto",
		},
		{
			in:       encodingZipkinProto,
			expected: "zipkin_proto",
		},
		{
			in:       encodingZipkinJSON,
			expected: "zipkin_json",
		},
		{
			in:       kafka.EncodingZipkinThrift,
			expected: "zipkin_thrift",
		},
		{
			in:           "not-an-encoding",
			expectsPanic: true,
		},
	}

	for _, tt := range tests {
		if tt.expectsPanic {
			assert.Panics(t, func() { mustOtelEncodingForJaegerEncoding(tt.in) })
			continue
		}

		assert.Equal(t, tt.expected, mustOtelEncodingForJaegerEncoding(tt.in))
	}
}
