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

package kafkaexporter

import (
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/configcheck"
	"go.opentelemetry.io/collector/config/configtest"
	otelKafkaExporter "go.opentelemetry.io/collector/exporter/kafkaexporter"

	"github.com/jaegertracing/jaeger/cmd/flags"
	jConfig "github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/plugin/storage/kafka"
)

func TestDefaultConfig(t *testing.T) {
	v, c := jConfig.Viperize(AddFlags)
	err := c.ParseFlags([]string{""})
	require.NoError(t, err)

	factory := &Factory{
		Wrapped: otelKafkaExporter.NewFactory(),
		Viper:   v,
	}
	defaultCfg := factory.CreateDefaultConfig().(*otelKafkaExporter.Config)

	assert.NoError(t, configcheck.ValidateConfig(defaultCfg))
	assert.Equal(t, "jaeger-spans", defaultCfg.Topic)
	assert.Equal(t, "jaeger_proto", defaultCfg.Encoding)
	assert.Equal(t, "", defaultCfg.ProtocolVersion)
	assert.Equal(t, []string{"127.0.0.1:9092"}, defaultCfg.Brokers)
	assert.Nil(t, defaultCfg.Authentication.Kerberos)
	assert.Nil(t, defaultCfg.Authentication.TLS)
	assert.Nil(t, defaultCfg.Authentication.PlainText)
}

func TestLoadConfigAndFlags(t *testing.T) {
	v, c := jConfig.Viperize(AddFlags, flags.AddConfigFileFlag)
	err := c.ParseFlags([]string{"--config-file=./testdata/jaeger-config.yaml", "--kafka.producer.topic=jaeger-test", "--kafka.producer.brokers=host1,host2"})
	require.NoError(t, err)

	err = flags.TryLoadConfigFile(v)
	require.NoError(t, err)

	factory := &Factory{
		Wrapped: otelKafkaExporter.NewFactory(),
		Viper:   v,
	}

	factories, err := componenttest.ExampleComponents()
	require.NoError(t, err)
	factories.Exporters[TypeStr] = factory
	cfg, err := configtest.LoadConfigFile(t, path.Join(".", "testdata", "config.yaml"), factories)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	kafkaCfg := cfg.Exporters[TypeStr].(*otelKafkaExporter.Config)
	require.NotNil(t, kafkaCfg)

	assert.Equal(t, TypeStr, kafkaCfg.Name())
	assert.Equal(t, "jaeger-prod", kafkaCfg.Topic)
	assert.Equal(t, "emojis", kafkaCfg.Encoding)
	assert.Equal(t, []string{"foo", "bar"}, kafkaCfg.Brokers)
	assert.Equal(t, "user", kafkaCfg.Authentication.PlainText.Username)
	assert.Equal(t, "123", kafkaCfg.Authentication.PlainText.Password)
	assert.Equal(t, "ca.crt", kafkaCfg.Authentication.TLS.CAFile)
	assert.Equal(t, "key.crt", kafkaCfg.Authentication.TLS.KeyFile)
	assert.Equal(t, "cert.crt", kafkaCfg.Authentication.TLS.CertFile)
	assert.Equal(t, true, kafkaCfg.Authentication.TLS.Insecure)
	assert.Equal(t, "jaeger", kafkaCfg.Authentication.Kerberos.Realm)
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
