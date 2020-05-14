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

package jaegerreceiver

import (
	"context"
	"fmt"
	"path"
	"testing"

	"github.com/open-telemetry/opentelemetry-collector/component"
	"github.com/open-telemetry/opentelemetry-collector/config"
	"github.com/open-telemetry/opentelemetry-collector/config/configerror"
	"github.com/open-telemetry/opentelemetry-collector/config/configmodels"
	"github.com/open-telemetry/opentelemetry-collector/receiver"
	"github.com/open-telemetry/opentelemetry-collector/receiver/jaegerreceiver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	collectorApp "github.com/jaegertracing/jaeger/cmd/collector/app"
	jConfig "github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/plugin/sampling/strategystore/static"
)

func TestDefaultValues(t *testing.T) {
	v, c := jConfig.Viperize(AddFlags)
	err := c.ParseFlags([]string{})
	require.NoError(t, err)

	factory := &Factory{Viper: v, Wrapped: &jaegerreceiver.Factory{}}
	cfg := factory.CreateDefaultConfig().(*jaegerreceiver.Config)
	assert.Nil(t, cfg.RemoteSampling)
	assert.Empty(t, cfg.Protocols)
}

func TestDefaultValueFromViper(t *testing.T) {
	tests := []struct {
		name     string
		flags    []string
		expected *jaegerreceiver.Config
	}{
		{
			name:  "samplingStrategyFile",
			flags: []string{fmt.Sprintf("--%s=%s", static.SamplingStrategiesFile, "conf.json")},
			expected: &jaegerreceiver.Config{
				RemoteSampling: &jaegerreceiver.RemoteSamplingConfig{
					StrategyFile: "conf.json",
				},
				Protocols: map[string]*receiver.SecureReceiverSettings{},
			},
		},
		{
			name:  "thriftCompact",
			flags: []string{fmt.Sprintf("--%s=%s", thriftCompactHostPort, "localhost:9999")},
			expected: &jaegerreceiver.Config{
				Protocols: map[string]*receiver.SecureReceiverSettings{
					"thrift_compact": {ReceiverSettings: configmodels.ReceiverSettings{Endpoint: "localhost:9999"}},
				},
			},
		},
		{
			name:  "thriftBinary",
			flags: []string{fmt.Sprintf("--%s=%s", thriftBinaryHostPort, "localhost:8888")},
			expected: &jaegerreceiver.Config{
				Protocols: map[string]*receiver.SecureReceiverSettings{
					"thrift_binary": {ReceiverSettings: configmodels.ReceiverSettings{Endpoint: "localhost:8888"}},
				},
			},
		},
		{
			name:  "grpc",
			flags: []string{fmt.Sprintf("--%s=%s", collectorApp.CollectorGRPCHostPort, "localhost:7894")},
			expected: &jaegerreceiver.Config{
				Protocols: map[string]*receiver.SecureReceiverSettings{
					"grpc": {ReceiverSettings: configmodels.ReceiverSettings{Endpoint: "localhost:7894"}},
				},
			},
		},
		{
			name:  "thriftHttp",
			flags: []string{fmt.Sprintf("--%s=%s", collectorApp.CollectorHTTPHostPort, "localhost:8080")},
			expected: &jaegerreceiver.Config{
				Protocols: map[string]*receiver.SecureReceiverSettings{
					"thrift_http": {ReceiverSettings: configmodels.ReceiverSettings{Endpoint: "localhost:8080"}},
				},
			},
		},
		{
			name:  "thriftHttpAndThriftBinary",
			flags: []string{fmt.Sprintf("--%s=%s", collectorApp.CollectorHTTPHostPort, "localhost:8089"), fmt.Sprintf("--%s=%s", thriftBinaryHostPort, "localhost:2222")},
			expected: &jaegerreceiver.Config{
				Protocols: map[string]*receiver.SecureReceiverSettings{
					"thrift_http":   {ReceiverSettings: configmodels.ReceiverSettings{Endpoint: "localhost:8089"}},
					"thrift_binary": {ReceiverSettings: configmodels.ReceiverSettings{Endpoint: "localhost:2222"}},
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			v, c := jConfig.Viperize(AddFlags)
			err := c.ParseFlags(test.flags)
			require.NoError(t, err)
			f := &Factory{
				Wrapped: &jaegerreceiver.Factory{},
				Viper:   v,
			}
			cfg := f.CreateDefaultConfig().(*jaegerreceiver.Config)
			test.expected.TypeVal = "jaeger"
			test.expected.NameVal = "jaeger"
			assert.Equal(t, test.expected, cfg)
		})
	}
}

func TestLoadConfigAndFlags(t *testing.T) {
	factories, err := config.ExampleComponents()
	require.NoError(t, err)

	v, c := jConfig.Viperize(AddFlags)
	err = c.ParseFlags([]string{"--sampling.strategies-file=bar.json"})
	require.NoError(t, err)

	factory := &Factory{Viper: v, Wrapped: &jaegerreceiver.Factory{}}
	assert.Equal(t, "bar.json", factory.CreateDefaultConfig().(*jaegerreceiver.Config).RemoteSampling.StrategyFile)

	factories.Receivers["jaeger"] = factory
	colConfig, err := config.LoadConfigFile(t, path.Join(".", "testdata", "config.yaml"), factories)
	require.NoError(t, err)
	require.NotNil(t, colConfig)

	cfg := colConfig.Receivers["jaeger"].(*jaegerreceiver.Config)
	assert.Equal(t, "foo.json", cfg.RemoteSampling.StrategyFile)
}

func TestType(t *testing.T) {
	f := &Factory{
		Wrapped: &jaegerreceiver.Factory{},
	}
	assert.Equal(t, configmodels.Type("jaeger"), f.Type())
}

func TestCreateMetricsExporter(t *testing.T) {
	f := &Factory{
		Wrapped: &jaegerreceiver.Factory{},
	}
	mReceiver, err := f.CreateMetricsReceiver(context.Background(), component.ReceiverCreateParams{}, nil, nil)
	assert.Equal(t, configerror.ErrDataTypeIsNotSupported, err)
	assert.Nil(t, mReceiver)
}
