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

package badgerexporter

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
	"github.com/jaegertracing/jaeger/plugin/storage/badger"
)

func TestDefaultConfig(t *testing.T) {
	factory := NewFactory(func() *badger.Options {
		opts := DefaultOptions()
		v, _ := jConfig.Viperize(opts.AddFlags)
		opts.InitFromViper(v)
		return opts
	})
	defaultCfg := factory.CreateDefaultConfig().(*Config)
	opts := defaultCfg.Options.GetPrimary()
	assert.Contains(t, opts.KeyDirectory, "/data/keys")
	assert.Contains(t, opts.ValueDirectory, "/data/values")
	assert.Equal(t, true, opts.Ephemeral)
	assert.Equal(t, false, opts.ReadOnly)
	assert.Equal(t, false, opts.SyncWrites)
	assert.Equal(t, false, opts.Truncate)
	assert.Equal(t, time.Second*10, opts.MetricsUpdateInterval)
	assert.Equal(t, time.Minute*5, opts.MaintenanceInterval)
	assert.Equal(t, time.Hour*72, opts.SpanStoreTTL)
}

func TestLoadConfigAndFlags(t *testing.T) {
	factories, err := componenttest.ExampleComponents()
	require.NoError(t, err)

	v, c := jConfig.Viperize(DefaultOptions().AddFlags)
	err = c.ParseFlags([]string{"--badger.directory-key=bar"})
	require.NoError(t, err)

	err = flags.TryLoadConfigFile(v)
	require.NoError(t, err)

	factory := NewFactory(func() *badger.Options {
		opts := DefaultOptions()
		opts.InitFromViper(v)
		require.Equal(t, "bar", opts.GetPrimary().KeyDirectory)
		return opts
	})

	factories.Exporters[TypeStr] = factory
	colConfig, err := configtest.LoadConfigFile(t, path.Join(".", "testdata", "config.yaml"), factories)
	require.NoError(t, err)
	require.NotNil(t, colConfig)

	cfg := colConfig.Exporters[TypeStr].(*Config)
	assert.Equal(t, TypeStr, cfg.Name())
	assert.Equal(t, "key", cfg.GetPrimary().KeyDirectory)
}
