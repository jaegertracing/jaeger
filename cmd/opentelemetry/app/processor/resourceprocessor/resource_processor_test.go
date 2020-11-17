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
	"context"
	"path"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/configmodels"
	"go.opentelemetry.io/collector/config/configtest"
	"go.opentelemetry.io/collector/processor/processorhelper"
	"go.opentelemetry.io/collector/processor/resourceprocessor"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/flags"
	jConfig "github.com/jaegertracing/jaeger/pkg/config"
)

func TestDefaultValues(t *testing.T) {
	v, c := jConfig.Viperize(AddFlags)
	err := c.ParseFlags([]string{})
	require.NoError(t, err)

	f := &Factory{Viper: v, Wrapped: resourceprocessor.NewFactory()}
	cfg := f.CreateDefaultConfig().(*resourceprocessor.Config)
	assert.Empty(t, cfg.Labels)
}

func TestDefaultValueFromViper(t *testing.T) {
	v, c := jConfig.Viperize(AddFlags)
	err := c.ParseFlags([]string{"--resource.attributes=foo=bar,orig=fake", "--jaeger.tags=foo=legacy,leg=head"})
	require.NoError(t, err)

	f := &Factory{
		Wrapped: resourceprocessor.NewFactory(),
		Viper:   v,
	}

	cfg := f.CreateDefaultConfig().(*resourceprocessor.Config)
	p, err := f.CreateTracesProcessor(context.Background(), component.ProcessorCreateParams{Logger: zap.NewNop()}, cfg, &componenttest.ExampleExporterConsumer{})
	require.NoError(t, err)
	assert.NotNil(t, p)

	sort.Slice(cfg.AttributesActions, func(i, j int) bool {
		return strings.Compare(cfg.AttributesActions[i].Key, cfg.AttributesActions[j].Key) < 0
	})
	assert.Equal(t, []processorhelper.ActionKeyValue{
		{Key: "foo", Value: "bar", Action: processorhelper.UPSERT},
		{Key: "leg", Value: "head", Action: processorhelper.UPSERT},
		{Key: "orig", Value: "fake", Action: processorhelper.UPSERT},
	}, cfg.AttributesActions)
}

func TestLegacyJaegerTagsOnly(t *testing.T) {
	v, c := jConfig.Viperize(AddFlags)
	err := c.ParseFlags([]string{"--jaeger.tags=foo=legacy,leg=head"})
	require.NoError(t, err)

	f := &Factory{
		Wrapped: resourceprocessor.NewFactory(),
		Viper:   v,
	}

	cfg := f.CreateDefaultConfig().(*resourceprocessor.Config)
	p, err := f.CreateTracesProcessor(context.Background(), component.ProcessorCreateParams{Logger: zap.NewNop()}, cfg, &componenttest.ExampleExporterConsumer{})
	require.NoError(t, err)
	assert.NotNil(t, p)

	sort.Slice(cfg.AttributesActions, func(i, j int) bool {
		return strings.Compare(cfg.AttributesActions[i].Key, cfg.AttributesActions[j].Key) < 0
	})
	assert.Equal(t, []processorhelper.ActionKeyValue{
		{Key: "foo", Value: "legacy", Action: processorhelper.UPSERT},
		{Key: "leg", Value: "head", Action: processorhelper.UPSERT},
	}, cfg.AttributesActions)
}

func TestLoadConfigAndFlags(t *testing.T) {
	factories, err := componenttest.ExampleComponents()
	require.NoError(t, err)

	v, c := jConfig.Viperize(AddFlags, flags.AddConfigFileFlag)
	err = c.ParseFlags([]string{"--resource.attributes=foo=bar,zone=zone2"})
	require.NoError(t, err)

	err = flags.TryLoadConfigFile(v)
	require.NoError(t, err)

	f := &Factory{
		Viper:   v,
		Wrapped: resourceprocessor.NewFactory(),
	}

	factories.Processors[f.Type()] = f
	colConfig, err := configtest.LoadConfigFile(t, path.Join(".", "testdata", "config.yaml"), factories)
	require.NoError(t, err)
	require.NotNil(t, colConfig)

	cfg := colConfig.Processors[string(f.Type())].(*resourceprocessor.Config)
	p, err := f.CreateTracesProcessor(context.Background(), component.ProcessorCreateParams{Logger: zap.NewNop()}, cfg, &componenttest.ExampleExporterConsumer{})
	require.NoError(t, err)
	assert.NotNil(t, p)
	assert.Equal(t, []processorhelper.ActionKeyValue{
		{Key: "zone", Value: "zone1", Action: processorhelper.UPSERT},
		{Key: "foo", Value: "bar", Action: processorhelper.UPSERT},
	}, cfg.AttributesActions)
}

func TestType(t *testing.T) {
	f := &Factory{
		Wrapped: resourceprocessor.NewFactory(),
	}
	assert.Equal(t, configmodels.Type("resource"), f.Type())
}

func TestCreateMetricsProcessor(t *testing.T) {
	f := &Factory{
		Wrapped: resourceprocessor.NewFactory(),
	}
	mReceiver, err := f.CreateMetricsProcessor(context.Background(), component.ProcessorCreateParams{}, &resourceprocessor.Config{
		AttributesActions: []processorhelper.ActionKeyValue{{Key: "foo", Value: "val", Action: processorhelper.UPSERT}},
	}, &componenttest.ExampleExporterConsumer{})
	require.Nil(t, err)
	assert.NotNil(t, mReceiver)
}
