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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/configerror"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/config/configmodels"
	"go.opentelemetry.io/collector/config/confignet"
	"go.opentelemetry.io/collector/config/configtest"
	"go.opentelemetry.io/collector/config/configtls"
	"go.opentelemetry.io/collector/receiver/jaegerreceiver"

	collectorApp "github.com/jaegertracing/jaeger/cmd/collector/app"
	jConfig "github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/plugin/sampling/strategystore/static"
)

func TestDefaultValues(t *testing.T) {
	v, c := jConfig.Viperize(AddFlags)
	err := c.ParseFlags([]string{})
	require.NoError(t, err)

	factory := &Factory{Viper: v, Wrapped: jaegerreceiver.NewFactory()}
	cfg := factory.CreateDefaultConfig().(*jaegerreceiver.Config)
	assert.Nil(t, cfg.RemoteSampling)
	assert.Empty(t, cfg.Protocols.ThriftCompact)
	assert.Empty(t, cfg.Protocols.ThriftBinary)
	assert.Empty(t, cfg.Protocols.ThriftHTTP)
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
				Protocols: jaegerreceiver.Protocols{},
			},
		},
		{
			name:  "thriftCompact",
			flags: []string{fmt.Sprintf("--%s=%s", thriftCompactHostPort, "localhost:9999")},
			expected: &jaegerreceiver.Config{
				Protocols: jaegerreceiver.Protocols{
					ThriftCompact: &jaegerreceiver.ProtocolUDP{
						Endpoint:        "localhost:9999",
						ServerConfigUDP: jaegerreceiver.DefaultServerConfigUDP(),
					},
				},
			},
		},
		{
			name:  "thriftBinary",
			flags: []string{fmt.Sprintf("--%s=%s", thriftBinaryHostPort, "localhost:8888")},
			expected: &jaegerreceiver.Config{
				Protocols: jaegerreceiver.Protocols{
					ThriftBinary: &jaegerreceiver.ProtocolUDP{
						Endpoint:        "localhost:8888",
						ServerConfigUDP: jaegerreceiver.DefaultServerConfigUDP(),
					},
				},
			},
		},
		{
			name:  "grpc",
			flags: []string{fmt.Sprintf("--%s=%s", collectorApp.CollectorGRPCHostPort, "localhost:7894")},
			expected: &jaegerreceiver.Config{
				Protocols: jaegerreceiver.Protocols{
					GRPC: &configgrpc.GRPCServerSettings{
						NetAddr: confignet.NetAddr{
							Endpoint: "localhost:7894",
						},
					},
				},
			},
		},
		{
			name:  "thriftHttp",
			flags: []string{fmt.Sprintf("--%s=%s", collectorApp.CollectorHTTPHostPort, "localhost:8080")},
			expected: &jaegerreceiver.Config{
				Protocols: jaegerreceiver.Protocols{
					ThriftHTTP: &confighttp.HTTPServerSettings{
						Endpoint: "localhost:8080",
					},
				},
			},
		},
		{
			name:  "thriftHttpAndThriftBinary",
			flags: []string{fmt.Sprintf("--%s=%s", collectorApp.CollectorHTTPHostPort, "localhost:8089"), fmt.Sprintf("--%s=%s", thriftBinaryHostPort, "localhost:2222")},
			expected: &jaegerreceiver.Config{
				Protocols: jaegerreceiver.Protocols{
					ThriftHTTP: &confighttp.HTTPServerSettings{
						Endpoint: "localhost:8089",
					},
					ThriftBinary: &jaegerreceiver.ProtocolUDP{
						Endpoint:        "localhost:2222",
						ServerConfigUDP: jaegerreceiver.DefaultServerConfigUDP(),
					},
				},
			},
		},
		{
			name: "remoteSampling",
			flags: []string{
				"--http-server.host-port=machine:1",
				"--sampling.strategies-file=foo",
				"--reporter.grpc.host-port=coll:33",
				"--reporter.grpc.tls.enabled=true",
				"--reporter.grpc.tls.ca=cacert.pem",
				"--reporter.grpc.tls.cert=cert.pem",
				"--reporter.grpc.tls.key=key.key",
			},
			expected: &jaegerreceiver.Config{
				RemoteSampling: &jaegerreceiver.RemoteSamplingConfig{
					StrategyFile: "foo",
					HostEndpoint: "machine:1",
					GRPCClientSettings: configgrpc.GRPCClientSettings{
						Endpoint: "coll:33",
						TLSSetting: configtls.TLSClientSetting{
							Insecure: false,
							TLSSetting: configtls.TLSSetting{
								CAFile:   "cacert.pem",
								CertFile: "cert.pem",
								KeyFile:  "key.key",
							},
						},
					},
				},
				Protocols: jaegerreceiver.Protocols{},
			},
		},
		{
			name: "collectorTLS",
			flags: []string{
				"--collector.grpc.tls.enabled=true",
				"--collector.grpc.tls.cert=/cert.pem",
				"--collector.grpc.tls.key=/key.pem",
				"--collector.grpc.tls.client-ca=/client-ca.pem",
			},
			expected: &jaegerreceiver.Config{
				Protocols: jaegerreceiver.Protocols{
					GRPC: &configgrpc.GRPCServerSettings{
						NetAddr: confignet.NetAddr{
							Endpoint: ":14250",
						},
						TLSSetting: &configtls.TLSServerSetting{
							TLSSetting: configtls.TLSSetting{
								CertFile: "/cert.pem",
								KeyFile:  "/key.pem",
							},
							ClientCAFile: "/client-ca.pem",
						},
					},
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
				Wrapped: jaegerreceiver.NewFactory(),
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
	factories, err := componenttest.ExampleComponents()
	require.NoError(t, err)

	v, c := jConfig.Viperize(AddFlags)
	err = c.ParseFlags([]string{"--sampling.strategies-file=bar.json"})
	require.NoError(t, err)

	factory := &Factory{Viper: v, Wrapped: jaegerreceiver.NewFactory()}
	assert.Equal(t, "bar.json", factory.CreateDefaultConfig().(*jaegerreceiver.Config).RemoteSampling.StrategyFile)

	factories.Receivers["jaeger"] = factory
	colConfig, err := configtest.LoadConfigFile(t, path.Join(".", "testdata", "config.yaml"), factories)
	require.NoError(t, err)
	require.NotNil(t, colConfig)

	cfg := colConfig.Receivers["jaeger"].(*jaegerreceiver.Config)
	assert.Equal(t, "foo.json", cfg.RemoteSampling.StrategyFile)
}

func TestType(t *testing.T) {
	f := &Factory{
		Wrapped: jaegerreceiver.NewFactory(),
	}
	assert.Equal(t, configmodels.Type("jaeger"), f.Type())
}

func TestCreateMetricsExporter(t *testing.T) {
	f := &Factory{
		Wrapped: jaegerreceiver.NewFactory(),
	}
	mReceiver, err := f.CreateMetricsReceiver(context.Background(), component.ReceiverCreateParams{}, nil, nil)
	assert.Equal(t, configerror.ErrDataTypeIsNotSupported, err)
	assert.Nil(t, mReceiver)
}
