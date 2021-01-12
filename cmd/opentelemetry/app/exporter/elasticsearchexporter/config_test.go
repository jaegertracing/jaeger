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

package elasticsearchexporter

import (
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/configtest"

	"github.com/jaegertracing/jaeger/cmd/flags"
	jConfig "github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/plugin/storage/es"
)

func TestDefaultConfig(t *testing.T) {
	v, _ := jConfig.Viperize(DefaultOptions().AddFlags)
	opts := DefaultOptions()
	opts.InitFromViper(v)
	factory := &Factory{OptionsFactory: func() *es.Options {
		return opts
	}}
	defaultCfg := factory.CreateDefaultConfig().(*Config)
	assert.Equal(t, []string{"http://127.0.0.1:9200"}, defaultCfg.GetPrimary().Servers)
	assert.Equal(t, int64(5), defaultCfg.GetPrimary().NumShards)
	assert.Equal(t, int64(1), defaultCfg.GetPrimary().NumReplicas)
	assert.Equal(t, "@", defaultCfg.GetPrimary().Tags.DotReplacement)
	assert.Equal(t, false, defaultCfg.GetPrimary().TLS.Enabled)
}

func TestLoadConfigAndFlags(t *testing.T) {
	factories, err := componenttest.ExampleComponents()
	require.NoError(t, err)

	v, c := jConfig.Viperize(DefaultOptions().AddFlags, flags.AddConfigFileFlag)
	err = c.ParseFlags([]string{"--es.server-urls=bar", "--es.index-prefix=staging", "--es.index-date-separator=-", "--config-file=./testdata/jaeger-config.yaml"})
	require.NoError(t, err)

	err = flags.TryLoadConfigFile(v)
	require.NoError(t, err)

	factory := &Factory{OptionsFactory: func() *es.Options {
		opts := DefaultOptions()
		opts.InitFromViper(v)
		require.Equal(t, []string{"bar"}, opts.GetPrimary().Servers)
		require.Equal(t, "staging", opts.GetPrimary().GetIndexPrefix())
		assert.Equal(t, int64(100), opts.GetPrimary().NumShards)
		return opts
	}}

	factories.Exporters[TypeStr] = factory
	colConfig, err := configtest.LoadConfigFile(t, path.Join(".", "testdata", "config.yaml"), factories)
	require.NoError(t, err)
	require.NotNil(t, colConfig)

	cfg := colConfig.Exporters[TypeStr].(*Config)
	esCfg := cfg.GetPrimary()
	assert.Equal(t, TypeStr, cfg.Name())
	assert.Equal(t, []string{"someUrl"}, esCfg.Servers)
	assert.Equal(t, true, esCfg.CreateIndexTemplates)
	assert.Equal(t, "staging", esCfg.IndexPrefix)
	assert.Equal(t, "2006-01-02", esCfg.IndexDateLayout)
	assert.Equal(t, int64(100), esCfg.NumShards)
	assert.Equal(t, "user", esCfg.Username)
	assert.Equal(t, "pass", esCfg.Password)
	assert.Equal(t, "/var/run/k8s", esCfg.TokenFilePath)
	assert.Equal(t, true, esCfg.UseReadWriteAliases)
	assert.Equal(t, true, esCfg.Sniffer)
	assert.Equal(t, true, esCfg.Tags.AllAsFields)
	assert.Equal(t, "/etc/jaeger", esCfg.Tags.File)
	assert.Equal(t, "O", esCfg.Tags.DotReplacement)
}
