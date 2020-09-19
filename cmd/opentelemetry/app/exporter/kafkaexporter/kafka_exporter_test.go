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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configcheck"
	otelKafkaExporter "go.opentelemetry.io/collector/exporter/kafkaexporter"

	"github.com/jaegertracing/jaeger/cmd/flags"
	jConfig "github.com/jaegertracing/jaeger/pkg/config"
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
	defaultCfg := factory.CreateDefaultConfig().(*otelKafkaExporter.Config)

	assert.Equal(t, TypeStr, defaultCfg.Name())
	assert.Equal(t, "jaeger-test", defaultCfg.Topic)
	assert.Equal(t, "jaeger_proto", defaultCfg.Encoding)
	assert.Equal(t, []string{"host1", "host2"}, defaultCfg.Brokers)
	assert.Equal(t, "jaeger", defaultCfg.Authentication.Kerberos.Realm)
	assert.Equal(t, "/etc/krb5.conf", defaultCfg.Authentication.Kerberos.ConfigPath)
	assert.Equal(t, "from-jaeger-config", defaultCfg.Authentication.Kerberos.Username)
}
