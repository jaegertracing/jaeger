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

	"github.com/imdario/mergo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/configcheck"
	"go.opentelemetry.io/collector/config/configmodels"
	"go.opentelemetry.io/collector/config/configtest"
	otelKafkaReceiver "go.opentelemetry.io/collector/receiver/kafkareceiver"

	"github.com/jaegertracing/jaeger/cmd/flags"
	"github.com/jaegertracing/jaeger/cmd/ingester/app"
	jConfig "github.com/jaegertracing/jaeger/pkg/config"
)

func TestDefaultConfig(t *testing.T) {
	v, c := jConfig.Viperize(app.AddFlags)
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
	assert.Nil(t, defaultCfg.Authentication.Kerberos)
	assert.Nil(t, defaultCfg.Authentication.TLS)
	assert.Nil(t, defaultCfg.Authentication.PlainText)

}

func TestLoadConfigAndFlags(t *testing.T) {
	factories, err := componenttest.ExampleComponents()
	require.NoError(t, err)

	v, c := jConfig.Viperize(app.AddFlags, flags.AddConfigFileFlag)
	err = c.ParseFlags([]string{"--config-file=./testdata/jaeger-config.yaml", "--kafka.consumer.topic=jaeger-test", "--kafka.consumer.brokers=host1,host2", "--kafka.consumer.group-id=from-flag"})
	require.NoError(t, err)

	err = flags.TryLoadConfigFile(v)
	require.NoError(t, err)

	factory := &Factory{
		Wrapped: otelKafkaReceiver.NewFactory(),
		Viper:   v,
	}
	fromJaegerCfg := &configmodels.Config{
		Receivers: configmodels.Receivers{
			TypeStr: factory.CreateDefaultConfig(),
		},
	}

	factories.Receivers[TypeStr] = factory
	fromOtelCfg, err := configtest.LoadConfigFile(t, path.Join(".", "testdata", "config.yaml"), factories)
	require.NoError(t, err)
	require.NotNil(t, fromOtelCfg)

	// defaultconfig.Merge() cannot be used b/c it creates an import cycle
	err = mergo.Merge(fromJaegerCfg, fromOtelCfg, mergo.WithOverride)
	require.NoError(t, err)

	defaultCfg := fromJaegerCfg.Receivers[TypeStr].(*otelKafkaReceiver.Config)

	assert.Equal(t, TypeStr, defaultCfg.Name())
	assert.Equal(t, "jaeger-prod", defaultCfg.Topic)
	assert.Equal(t, "emojis", defaultCfg.Encoding)
	assert.Equal(t, []string{"foo", "bar"}, defaultCfg.Brokers)
	assert.Equal(t, "from-flag", defaultCfg.GroupID)
	assert.Equal(t, "jaeger-ingester", defaultCfg.ClientID)
	assert.Equal(t, "user", defaultCfg.Authentication.PlainText.Username)
	assert.Equal(t, "123", defaultCfg.Authentication.PlainText.Password)
	assert.Equal(t, "ca.crt", defaultCfg.Authentication.TLS.CAFile)
	assert.Equal(t, "key.crt", defaultCfg.Authentication.TLS.KeyFile)
	assert.Equal(t, true, defaultCfg.Authentication.TLS.Insecure)
	assert.Equal(t, "jaeger", defaultCfg.Authentication.Kerberos.Realm)
	assert.Equal(t, "/etc/foo", defaultCfg.Authentication.Kerberos.ConfigPath)
	assert.Equal(t, "from-jaeger-config", defaultCfg.Authentication.Kerberos.Username)
}
