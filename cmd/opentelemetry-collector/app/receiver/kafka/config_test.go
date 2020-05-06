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

	"github.com/open-telemetry/opentelemetry-collector/config"
	"github.com/open-telemetry/opentelemetry-collector/config/configcheck"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/cmd/flags"
	"github.com/jaegertracing/jaeger/cmd/ingester/app"
	jConfig "github.com/jaegertracing/jaeger/pkg/config"
)

func TestDefaultConfig(t *testing.T) {
	v, c := jConfig.Viperize(app.AddFlags)
	err := c.ParseFlags([]string{""})
	require.NoError(t, err)
	factory := &Factory{OptionsFactory: func() *app.Options {
		opts := DefaultOptions()
		opts.InitFromViper(v)
		return opts
	}}
	defaultCfg := factory.CreateDefaultConfig().(*Config)
	assert.NoError(t, configcheck.ValidateConfig(defaultCfg))
	assert.Equal(t, "jaeger-spans", defaultCfg.Topic)
	assert.Equal(t, "protobuf", defaultCfg.Encoding)
	assert.Equal(t, []string{"127.0.0.1:9092"}, defaultCfg.Brokers)
	assert.Equal(t, "none", defaultCfg.Authentication)
	assert.Equal(t, "/etc/krb5.conf", defaultCfg.Kerberos.ConfigPath)
	assert.Equal(t, "kafka", defaultCfg.Kerberos.ServiceName)
	assert.Equal(t, false, defaultCfg.TLS.Enabled)
}

func TestLoadConfigAndFlags(t *testing.T) {
	factories, err := config.ExampleComponents()
	require.NoError(t, err)

	v, c := jConfig.Viperize(app.AddFlags, flags.AddConfigFileFlag)
	err = c.ParseFlags([]string{"--config-file=./testdata/jaeger-config.yaml", "--kafka.consumer.topic=jaeger-test", "--kafka.consumer.brokers=host1,host2"})
	require.NoError(t, err)

	err = flags.TryLoadConfigFile(v)
	require.NoError(t, err)

	factory := &Factory{OptionsFactory: func() *app.Options {
		opts := DefaultOptions()
		opts.InitFromViper(v)
		assert.Equal(t, "jaeger-test", opts.Topic)
		assert.Equal(t, []string{"host1", "host2"}, opts.Brokers)
		return opts
	}}

	factories.Receivers[TypeStr] = factory
	cfg, err := config.LoadConfigFile(t, path.Join(".", "testdata", "config.yaml"), factories)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	kafkaCfg := cfg.Receivers[TypeStr].(*Config)
	assert.Equal(t, TypeStr, kafkaCfg.Name())
	assert.Equal(t, "jaeger-prod", kafkaCfg.Topic)
	assert.Equal(t, "emojis", kafkaCfg.Encoding)
	assert.Equal(t, []string{"foo", "bar"}, kafkaCfg.Options.Brokers)
	assert.Equal(t, "tls", kafkaCfg.Options.Authentication)
	assert.Equal(t, "user", kafkaCfg.Options.PlainText.UserName)
	assert.Equal(t, "123", kafkaCfg.Options.PlainText.Password)
	assert.Equal(t, true, kafkaCfg.Options.TLS.Enabled)
	assert.Equal(t, "ca.crt", kafkaCfg.Options.TLS.CAPath)
	assert.Equal(t, "key.crt", kafkaCfg.Options.TLS.KeyPath)
	assert.Equal(t, "cert.crt", kafkaCfg.Options.TLS.CertPath)
	assert.Equal(t, true, kafkaCfg.Options.TLS.SkipHostVerify)
	assert.Equal(t, "jaeger", kafkaCfg.Options.Kerberos.Realm)
	assert.Equal(t, "/etc/foo", kafkaCfg.Options.Kerberos.ConfigPath)
	assert.Equal(t, "from-jaeger-config", kafkaCfg.Options.Kerberos.Username)
}
