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

package resourceprocessor

import (
	"path"
	"testing"

	"github.com/open-telemetry/opentelemetry-collector/config"
	"github.com/open-telemetry/opentelemetry-collector/config/configmodels"
	"github.com/open-telemetry/opentelemetry-collector/processor/resourceprocessor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/flags"
	jConfig "github.com/jaegertracing/jaeger/pkg/config"
)

func TestDefaultValues(t *testing.T) {
	v, c := jConfig.Viperize(AddFlags)
	err := c.ParseFlags([]string{})
	require.NoError(t, err)

	f := &Factory{Viper: v, Wrapped: &resourceprocessor.Factory{}}
	cfg := f.CreateDefaultConfig().(*resourceprocessor.Config)
	assert.Empty(t, cfg.Labels)
}

func TestDefaultValueFromViper(t *testing.T) {
	v, c := jConfig.Viperize(AddFlags)
	err := c.ParseFlags([]string{"--resource.labels=foo=bar,orig=fake", "--jaeger.tags=foo=legacy,leg=head"})
	require.NoError(t, err)

	f := &Factory{
		Wrapped: &resourceprocessor.Factory{},
		Viper:   v,
	}

	cfg := f.CreateDefaultConfig().(*resourceprocessor.Config)
	assert.Equal(t, map[string]string{"foo": "bar", "leg": "head", "orig": "fake"}, cfg.Labels)
}

func TestLoadConfigAndFlags(t *testing.T) {
	factories, err := config.ExampleComponents()
	require.NoError(t, err)

	v, c := jConfig.Viperize(AddFlags, flags.AddConfigFileFlag)
	err = c.ParseFlags([]string{"--resource.labels=foo=bar,zone=zone2"})
	require.NoError(t, err)

	err = flags.TryLoadConfigFile(v)
	require.NoError(t, err)

	f := &Factory{
		Viper:   v,
		Wrapped: &resourceprocessor.Factory{},
	}

	factories.Processors[f.Type()] = f
	colConfig, err := config.LoadConfigFile(t, path.Join(".", "testdata", "config.yaml"), factories)
	require.NoError(t, err)
	require.NotNil(t, colConfig)

	cfg := colConfig.Processors[string(f.Type())].(*resourceprocessor.Config)
	assert.Equal(t, map[string]string{"zone": "zone1", "foo": "bar"}, cfg.Labels)
	p, err := f.CreateTraceProcessor(zap.NewNop(), nil, cfg)
	require.NoError(t, err)
	assert.NotNil(t, p)
}

func TestType(t *testing.T) {
	f := &Factory{
		Wrapped: &resourceprocessor.Factory{},
	}
	assert.Equal(t, configmodels.Type("resource"), f.Type())
}

func TestCreateMetricsExporter(t *testing.T) {
	f := &Factory{
		Wrapped: &resourceprocessor.Factory{},
	}
	mReceiver, err := f.CreateMetricsProcessor(zap.NewNop(), nil, &resourceprocessor.Config{})
	require.Nil(t, err)
	assert.NotNil(t, mReceiver)
}
