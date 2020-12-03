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

package jaegerexporter

import (
	"context"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/configerror"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/configmodels"
	"go.opentelemetry.io/collector/config/configtest"
	"go.opentelemetry.io/collector/config/configtls"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.opentelemetry.io/collector/exporter/jaegerexporter"

	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/receiver/jaegerreceiver"
	jConfig "github.com/jaegertracing/jaeger/pkg/config"
)

func TestDefaultValues(t *testing.T) {
	v, c := jConfig.Viperize(jaegerreceiver.AddFlags)
	err := c.ParseFlags([]string{})
	require.NoError(t, err)

	factory := &Factory{Viper: v, Wrapped: jaegerexporter.NewFactory()}
	cfg := factory.CreateDefaultConfig().(*jaegerexporter.Config)
	assert.Empty(t, cfg.GRPCClientSettings.Endpoint)
	tlsConf := cfg.TLSSetting
	assert.True(t, tlsConf.Insecure)
	assert.Empty(t, tlsConf.CAFile)
	assert.Empty(t, tlsConf.KeyFile)
	assert.Empty(t, tlsConf.CertFile)
	assert.Empty(t, tlsConf.ServerName)
}

func TestDefaultValueFromViper(t *testing.T) {
	v, c := jConfig.Viperize(jaegerreceiver.AddFlags)
	err := c.ParseFlags([]string{"--reporter.grpc.host-port=foo", "--reporter.grpc.tls.enabled=true", "--reporter.grpc.tls.ca=ca.crt"})
	require.NoError(t, err)

	f := &Factory{
		Wrapped: jaegerexporter.NewFactory(),
		Viper:   v,
	}

	cfg := f.CreateDefaultConfig().(*jaegerexporter.Config)

	qs := exporterhelper.DefaultQueueSettings()
	assert.Equal(t, &jaegerexporter.Config{
		ExporterSettings: configmodels.ExporterSettings{
			TypeVal: "jaeger",
			NameVal: "jaeger",
		},
		TimeoutSettings: exporterhelper.DefaultTimeoutSettings(),
		RetrySettings:   exporterhelper.DefaultRetrySettings(),
		QueueSettings:   qs,
		GRPCClientSettings: configgrpc.GRPCClientSettings{
			WriteBufferSize: 512 * 1024,
			Endpoint:        "foo",
			TLSSetting: configtls.TLSClientSetting{
				Insecure: false,
				TLSSetting: configtls.TLSSetting{
					CAFile: "ca.crt",
				},
			},
		},
	}, cfg)
}

func TestLoadConfigAndFlags(t *testing.T) {
	factories, err := componenttest.ExampleComponents()
	require.NoError(t, err)

	v, c := jConfig.Viperize(jaegerreceiver.AddFlags)
	err = c.ParseFlags([]string{"--reporter.grpc.host-port=foo"})
	require.NoError(t, err)

	factory := &Factory{Viper: v, Wrapped: jaegerexporter.NewFactory()}
	assert.Equal(t, "foo", factory.CreateDefaultConfig().(*jaegerexporter.Config).GRPCClientSettings.Endpoint)

	factories.Exporters["jaeger"] = factory
	colConfig, err := configtest.LoadConfigFile(t, path.Join(".", "testdata", "config.yaml"), factories)
	require.NoError(t, err)
	require.NotNil(t, colConfig)

	cfg := colConfig.Exporters["jaeger"].(*jaegerexporter.Config)
	assert.Equal(t, "bar", cfg.GRPCClientSettings.Endpoint)
}

func TestType(t *testing.T) {
	f := &Factory{
		Wrapped: jaegerexporter.NewFactory(),
	}
	assert.Equal(t, configmodels.Type("jaeger"), f.Type())
}

func TestCreateMetricsExporter(t *testing.T) {
	f := &Factory{
		Wrapped: jaegerexporter.NewFactory(),
	}
	mReceiver, err := f.CreateMetricsExporter(context.Background(), component.ExporterCreateParams{}, nil)
	assert.Equal(t, configerror.ErrDataTypeIsNotSupported, err)
	assert.Nil(t, mReceiver)
}
