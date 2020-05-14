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

	"github.com/open-telemetry/opentelemetry-collector/component"
	"github.com/open-telemetry/opentelemetry-collector/config/configmodels"
	"github.com/open-telemetry/opentelemetry-collector/consumer"
	"github.com/open-telemetry/opentelemetry-collector/receiver"
	"github.com/open-telemetry/opentelemetry-collector/receiver/jaegerreceiver"
	"github.com/spf13/viper"

	agentApp "github.com/jaegertracing/jaeger/cmd/agent/app"
	grpcRep "github.com/jaegertracing/jaeger/cmd/agent/app/reporter/grpc"
	collectorApp "github.com/jaegertracing/jaeger/cmd/collector/app"
	"github.com/jaegertracing/jaeger/plugin/sampling/strategystore/static"
)

// Factory wraps jaegerreceiver.Factory and makes the default config configurable via viper.
// For instance this enables using flags as default values in the config object.
type Factory struct {
	// Wrapped is Jaeger receiver.
	Wrapped *jaegerreceiver.Factory
	// Viper is used to get configuration values for default configuration
	Viper *viper.Viper
}

var _ component.ReceiverFactory = (*Factory)(nil)

// Type returns the type of the receiver.
func (f *Factory) Type() configmodels.Type {
	return f.Wrapped.Type()
}

// CreateDefaultConfig returns default configuration of Factory.
// This function implements OTEL component.ReceiverFactoryBase interface.
func (f *Factory) CreateDefaultConfig() configmodels.Receiver {
	cfg := f.Wrapped.CreateDefaultConfig().(*jaegerreceiver.Config)
	cfg.RemoteSampling = createDefaultSamplingConfig(f.Viper)
	configureAgent(f.Viper, cfg)
	configureCollector(f.Viper, cfg)
	return cfg
}

func configureAgent(v *viper.Viper, cfg *jaegerreceiver.Config) {
	aOpts := agentApp.Builder{}
	aOpts.InitFromViper(v)
	if v.IsSet(thriftBinaryHostPort) {
		cfg.Protocols["thrift_binary"] = &receiver.SecureReceiverSettings{
			ReceiverSettings: configmodels.ReceiverSettings{
				// TODO OTEL does not expose number of workers or queue length
				Endpoint: v.GetString(thriftBinaryHostPort),
			},
		}
	}
	if v.IsSet(thriftCompactHostPort) {
		cfg.Protocols["thrift_compact"] = &receiver.SecureReceiverSettings{
			ReceiverSettings: configmodels.ReceiverSettings{
				// TODO OTEL does not expose number of workers or queue length
				Endpoint: v.GetString(thriftCompactHostPort),
			},
		}
	}
}

func configureCollector(v *viper.Viper, cfg *jaegerreceiver.Config) {
	cOpts := collectorApp.CollectorOptions{}
	cOpts.InitFromViper(v)
	if v.IsSet(collectorApp.CollectorGRPCHostPort) {
		cfg.Protocols["grpc"] = &receiver.SecureReceiverSettings{
			ReceiverSettings: configmodels.ReceiverSettings{
				Endpoint: cOpts.CollectorGRPCHostPort,
			},
		}
		if cOpts.TLS.CertPath != "" && cOpts.TLS.KeyPath != "" {
			cfg.Protocols["grpc"].TLSCredentials = &receiver.TLSCredentials{
				// TODO client-ca is missing in OTEL
				KeyFile:  cOpts.TLS.KeyPath,
				CertFile: cOpts.TLS.CertPath,
			}
		}
	}
	if v.IsSet(collectorApp.CollectorHTTPHostPort) {
		cfg.Protocols["thrift_http"] = &receiver.SecureReceiverSettings{
			ReceiverSettings: configmodels.ReceiverSettings{
				Endpoint: cOpts.CollectorHTTPHostPort,
			},
		}
	}
}

func createDefaultSamplingConfig(v *viper.Viper) *jaegerreceiver.RemoteSamplingConfig {
	var samplingConf *jaegerreceiver.RemoteSamplingConfig
	strategyFile := v.GetString(static.SamplingStrategiesFile)
	if strategyFile != "" {
		samplingConf = &jaegerreceiver.RemoteSamplingConfig{
			StrategyFile: strategyFile,
		}
	}
	repCfg := grpcRep.ConnBuilder{}
	repCfg.InitFromViper(v)
	// This is for agent mode.
	// This uses --reporter.grpc.host-port flag to set the fetch endpoint for the sampling strategies.
	// The same flag is used by Jaeger exporter. If the value is not provided Jaeger exporter fails to start.
	if len(repCfg.CollectorHostPorts) > 0 {
		if samplingConf == nil {
			samplingConf = &jaegerreceiver.RemoteSamplingConfig{}
		}
		samplingConf.GRPCSettings.Endpoint = repCfg.CollectorHostPorts[0]
		samplingConf.GRPCSettings.TLSConfig.UseSecure = repCfg.TLS.Enabled
		samplingConf.GRPCSettings.TLSConfig.CaCert = repCfg.TLS.CAPath
		samplingConf.GRPCSettings.TLSConfig.ClientCert = repCfg.TLS.CertPath
		samplingConf.GRPCSettings.TLSConfig.ClientKey = repCfg.TLS.KeyPath
		samplingConf.GRPCSettings.TLSConfig.ServerNameOverride = repCfg.TLS.ServerName
	}
	return samplingConf
}

// CreateTraceReceiver creates Jaeger receiver trace receiver.
// This function implements OTEL component.ReceiverFactory interface.
func (f *Factory) CreateTraceReceiver(
	ctx context.Context,
	params component.ReceiverCreateParams,
	cfg configmodels.Receiver,
	nextConsumer consumer.TraceConsumer,
) (component.TraceReceiver, error) {
	return f.Wrapped.CreateTraceReceiver(ctx, params, cfg, nextConsumer)
}

// CustomUnmarshaler creates custom unmarshaller for Jaeger receiver config.
// This function implements component.ReceiverFactoryBase interface.
func (f *Factory) CustomUnmarshaler() component.CustomUnmarshaler {
	return f.Wrapped.CustomUnmarshaler()
}

// CreateMetricsReceiver creates a metrics receiver based on provided config.
// This function implements component.ReceiverFactory.
func (f *Factory) CreateMetricsReceiver(
	ctx context.Context,
	params component.ReceiverCreateParams,
	cfg configmodels.Receiver,
	nextConsumer consumer.MetricsConsumer,
) (component.MetricsReceiver, error) {
	return f.Wrapped.CreateMetricsReceiver(ctx, params, cfg, nextConsumer)
}
