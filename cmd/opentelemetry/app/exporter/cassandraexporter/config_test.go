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

package cassandraexporter

import (
	"path"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/configtest"

	"github.com/jaegertracing/jaeger/cmd/flags"
	jConfig "github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/plugin/storage/cassandra"
)

func TestDefaultConfig(t *testing.T) {
	factory := &Factory{OptionsFactory: func() *cassandra.Options {
		v, _ := jConfig.Viperize(DefaultOptions().AddFlags)
		opts := DefaultOptions()
		opts.InitFromViper(v)
		return opts
	}}
	defaultCfg := factory.CreateDefaultConfig().(*Config)
	assert.Equal(t, []string{"127.0.0.1"}, defaultCfg.Options.GetPrimary().Servers)
	assert.Equal(t, []string{"127.0.0.1"}, defaultCfg.Options.Primary.Servers)
	assert.Equal(t, 2, defaultCfg.Primary.ConnectionsPerHost)
	assert.Equal(t, "jaeger_v1_test", defaultCfg.Primary.Keyspace)
	assert.Equal(t, 3, defaultCfg.Primary.MaxRetryAttempts)
	assert.Equal(t, 4, defaultCfg.Primary.ProtoVersion)
	assert.Equal(t, time.Minute, defaultCfg.Primary.ReconnectInterval)
	assert.Equal(t, time.Hour*12, defaultCfg.SpanStoreWriteCacheTTL)
	assert.Equal(t, true, defaultCfg.Index.Tags)
	assert.Equal(t, true, defaultCfg.Index.Logs)
	assert.Equal(t, true, defaultCfg.Index.ProcessTags)
}

func TestLoadConfigAndFlags(t *testing.T) {
	factories, err := componenttest.ExampleComponents()
	require.NoError(t, err)

	v, c := jConfig.Viperize(DefaultOptions().AddFlags, flags.AddConfigFileFlag)
	err = c.ParseFlags([]string{"--cassandra.servers=bar", "--cassandra.port=9000", "--config-file=./testdata/jaeger-config.yaml"})
	require.NoError(t, err)

	err = flags.TryLoadConfigFile(v)
	require.NoError(t, err)

	factory := &Factory{OptionsFactory: func() *cassandra.Options {
		opts := DefaultOptions()
		opts.InitFromViper(v)
		require.Equal(t, []string{"bar"}, opts.GetPrimary().Servers)
		return opts
	}}

	factories.Exporters[TypeStr] = factory
	colConfig, err := configtest.LoadConfigFile(t, path.Join(".", "testdata", "config.yaml"), factories)
	require.NoError(t, err)
	require.NotNil(t, colConfig)

	cfg := colConfig.Exporters[TypeStr].(*Config)
	assert.Equal(t, TypeStr, cfg.Name())
	assert.Equal(t, []string{"first", "second"}, cfg.Primary.Servers)
	assert.Equal(t, 9000, cfg.Primary.Port)
	assert.Equal(t, false, cfg.Index.Tags)
	assert.Equal(t, "my-keyspace", cfg.Primary.Keyspace)
	assert.Equal(t, false, cfg.Index.Tags)
	assert.Equal(t, true, cfg.Index.Logs)
	assert.Equal(t, "user", cfg.Primary.Authenticator.Basic.Username)
	assert.Equal(t, "pass", cfg.Primary.Authenticator.Basic.Password)
	assert.Equal(t, time.Second*12, cfg.SpanStoreWriteCacheTTL)
	assert.Equal(t, true, cfg.Primary.TLS.Enabled)
	assert.Equal(t, "/foo/bar", cfg.Primary.TLS.CAPath)
}
