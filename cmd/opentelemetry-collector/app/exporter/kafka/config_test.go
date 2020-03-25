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

package kafka

import (
	"path"
	"testing"

	"github.com/Shopify/sarama"
	"github.com/open-telemetry/opentelemetry-collector/config"
	"github.com/open-telemetry/opentelemetry-collector/config/configcheck"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/cmd/flags"
	jConfig "github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/plugin/storage/kafka"
)

func TestDefaultConfig(t *testing.T) {
	v, c := jConfig.Viperize(DefaultOptions().AddFlags)
	err := c.ParseFlags([]string{""})
	require.NoError(t, err)
	factory := &Factory{OptionsFactory: func() *kafka.Options {
		opts := DefaultOptions()
		opts.InitFromViper(v)
		return opts
	}}
	defaultCfg := factory.CreateDefaultConfig().(*Config)
	assert.NoError(t, configcheck.ValidateConfig(defaultCfg))
	assert.Equal(t, "jaeger-spans", defaultCfg.Topic)
	assert.Equal(t, "protobuf", defaultCfg.Encoding)
	assert.Equal(t, []string{"127.0.0.1:9092"}, defaultCfg.Config.Brokers)
	assert.Equal(t, sarama.WaitForLocal, defaultCfg.Config.RequiredAcks)
	assert.Equal(t, "none", defaultCfg.Config.Authentication)
	assert.Equal(t, "/etc/krb5.conf", defaultCfg.Config.Kerberos.ConfigPath)
	assert.Equal(t, "kafka", defaultCfg.Config.Kerberos.ServiceName)
	assert.Equal(t, false, defaultCfg.Config.TLS.Enabled)
}

func TestLoadConfigAndFlags(t *testing.T) {
	factories, err := config.ExampleComponents()
	require.NoError(t, err)

	v, c := jConfig.Viperize(DefaultOptions().AddFlags, flags.AddConfigFileFlag)
	err = c.ParseFlags([]string{"--config-file=./testdata/jaeger-config.yaml", "--kafka.producer.topic=jaeger-test", "--kafka.producer.brokers=host1,host2"})
	require.NoError(t, err)

	err = flags.TryLoadConfigFile(v)
	require.NoError(t, err)

	factory := &Factory{OptionsFactory: func() *kafka.Options {
		opts := DefaultOptions()
		opts.InitFromViper(v)
		assert.Equal(t, "jaeger-test", opts.Topic)
		assert.Equal(t, []string{"host1", "host2"}, opts.Config.Brokers)
		return opts
	}}

	factories.Exporters[TypeStr] = factory
	cfg, err := config.LoadConfigFile(t, path.Join(".", "testdata", "config.yaml"), factories)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	kafkaCfg := cfg.Exporters[TypeStr].(*Config)
	assert.Equal(t, TypeStr, kafkaCfg.Name())
	assert.Equal(t, "jaeger-prod", kafkaCfg.Topic)
	assert.Equal(t, "emojis", kafkaCfg.Encoding)
	assert.Equal(t, []string{"foo", "bar"}, kafkaCfg.Config.Brokers)
	assert.Equal(t, "tls", kafkaCfg.Config.Authentication)
	assert.Equal(t, "user", kafkaCfg.Config.PlainText.UserName)
	assert.Equal(t, "123", kafkaCfg.Config.PlainText.Password)
	assert.Equal(t, true, kafkaCfg.Config.TLS.Enabled)
	assert.Equal(t, "ca.crt", kafkaCfg.Config.TLS.CAPath)
	assert.Equal(t, "key.crt", kafkaCfg.Config.TLS.KeyPath)
	assert.Equal(t, "cert.crt", kafkaCfg.Config.TLS.CertPath)
	assert.Equal(t, true, kafkaCfg.Config.TLS.SkipHostVerify)
	assert.Equal(t, "jaeger", kafkaCfg.Config.Kerberos.Realm)
	assert.Equal(t, "/etc/foo", kafkaCfg.Config.Kerberos.ConfigPath)
	assert.Equal(t, "from-jaeger-config", kafkaCfg.Config.Kerberos.Username)
}
