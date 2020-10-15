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

package zipkinreceiver

import (
	"context"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/configerror"
	"go.opentelemetry.io/collector/config/configmodels"
	"go.opentelemetry.io/collector/config/configtest"
	"go.opentelemetry.io/collector/receiver/zipkinreceiver"

	jConfig "github.com/jaegertracing/jaeger/pkg/config"
)

func TestDefaultValues(t *testing.T) {
	v, c := jConfig.Viperize(AddFlags)
	err := c.ParseFlags([]string{})
	require.NoError(t, err)

	factory := &Factory{Viper: v, Wrapped: zipkinreceiver.NewFactory()}
	cfg := factory.CreateDefaultConfig().(*zipkinreceiver.Config)
	assert.Equal(t, "", cfg.Endpoint)
}

func TestLoadConfigAndFlags(t *testing.T) {
	factories, err := componenttest.ExampleComponents()
	require.NoError(t, err)

	v, c := jConfig.Viperize(AddFlags)
	err = c.ParseFlags([]string{"--collector.zipkin.host-port=bar:111"})
	require.NoError(t, err)

	factory := &Factory{Viper: v, Wrapped: zipkinreceiver.NewFactory()}
	assert.Equal(t, "bar:111", factory.CreateDefaultConfig().(*zipkinreceiver.Config).Endpoint)

	factories.Receivers["zipkin"] = factory
	colConfig, err := configtest.LoadConfigFile(t, path.Join(".", "testdata", "config.yaml"), factories)
	require.NoError(t, err)
	require.NotNil(t, colConfig)

	cfg := colConfig.Receivers["zipkin"].(*zipkinreceiver.Config)
	assert.Equal(t, "foo:9411", cfg.Endpoint)
}

func TestType(t *testing.T) {
	f := &Factory{
		Wrapped: zipkinreceiver.NewFactory(),
	}
	assert.Equal(t, configmodels.Type("zipkin"), f.Type())
}

func TestCreateMetricsExporter(t *testing.T) {
	f := &Factory{
		Wrapped: zipkinreceiver.NewFactory(),
	}
	mReceiver, err := f.CreateMetricsReceiver(context.Background(), component.ReceiverCreateParams{}, nil, nil)
	assert.Equal(t, configerror.ErrDataTypeIsNotSupported, err)
	assert.Nil(t, mReceiver)
}
