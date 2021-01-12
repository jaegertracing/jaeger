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

package memoryexporter

import (
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/configtest"

	"github.com/jaegertracing/jaeger/cmd/flags"
	jConfig "github.com/jaegertracing/jaeger/pkg/config"
)

func TestDefaultConfig(t *testing.T) {
	v, _ := jConfig.Viperize(AddFlags)
	factory := NewFactory(v)
	defaultCfg := factory.CreateDefaultConfig().(*Config)
	assert.Equal(t, 0, defaultCfg.Configuration.MaxTraces)
}

func TestLoadConfigAndFlags(t *testing.T) {
	factories, err := componenttest.ExampleComponents()
	require.NoError(t, err)

	v, c := jConfig.Viperize(AddFlags, flags.AddConfigFileFlag)
	err = c.ParseFlags([]string{"--memory.max-traces=15"})
	require.NoError(t, err)

	err = flags.TryLoadConfigFile(v)
	require.NoError(t, err)

	factory := NewFactory(v)
	assert.Equal(t, 15, factory.CreateDefaultConfig().(*Config).Configuration.MaxTraces)

	factories.Exporters[TypeStr] = factory
	colConfig, err := configtest.LoadConfigFile(t, path.Join(".", "testdata", "config.yaml"), factories)
	require.NoError(t, err)
	require.NotNil(t, colConfig)

	cfg := colConfig.Exporters[TypeStr].(*Config)
	memCfg := cfg.Configuration
	assert.Equal(t, TypeStr, cfg.Name())
	assert.Equal(t, 150, memCfg.MaxTraces)
}
