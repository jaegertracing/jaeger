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

	"github.com/spf13/viper"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/config/configmodels"
	"go.opentelemetry.io/collector/config/confignet"
	"go.opentelemetry.io/collector/config/configtls"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/receiver/jaegerreceiver"

	agentApp "github.com/jaegertracing/jaeger/cmd/agent/app"
	grpcRep "github.com/jaegertracing/jaeger/cmd/agent/app/reporter/grpc"
	collectorApp "github.com/jaegertracing/jaeger/cmd/collector/app"
	"github.com/jaegertracing/jaeger/plugin/sampling/strategystore/static"
	"github.com/jaegertracing/jaeger/ports"
)

// Factory wraps jaegerreceiver.Factory and makes the default config configurable via viper.
// For instance this enables using flags as default values in the config object.
type Factory struct {
	// Wrapped is Jaeger receiver.
	Wrapped component.ReceiverFactory
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
	// disable all ports by resetting the protocols
	// The custom unmarshaller sets unused protocols to nil, however it is only invoked when parsing the config
	// The config is not parsed in the default/hardcoded config, hence this fix is required
	cfg.Protocols = jaegerreceiver.Protocols{}
	cfg.RemoteSampling = createDefaultSamplingConfig(f.Viper)
	configureAgent(f.Viper, cfg)
	configureCollector(f.Viper, cfg)
	return cfg
}

func configureAgent(v *viper.Viper, cfg *jaegerreceiver.Config) {
	aOpts := agentApp.Builder{}
	aOpts.InitFromViper(v)
	if v.IsSet(thriftBinaryHostPort) {
		cfg.ThriftBinary = &jaegerreceiver.ProtocolUDP{
			Endpoint:        v.GetString(thriftBinaryHostPort),
			ServerConfigUDP: jaegerreceiver.DefaultServerConfigUDP(),
		}
	}
	if v.IsSet(thriftCompactHostPort) {
		cfg.ThriftCompact = &jaegerreceiver.ProtocolUDP{
			Endpoint:        v.GetString(thriftCompactHostPort),
			ServerConfigUDP: jaegerreceiver.DefaultServerConfigUDP(),
		}
	}
}

func configureCollector(v *viper.Viper, cfg *jaegerreceiver.Config) {
	cOpts := collectorApp.CollectorOptions{}
	cOpts.InitFromViper(v)
	if v.IsSet(collectorApp.CollectorGRPCHostPort) {
		cfg.GRPC = &configgrpc.GRPCServerSettings{
			NetAddr: confignet.NetAddr{
				Endpoint: cOpts.CollectorGRPCHostPort,
			},
		}
	}
	if cOpts.TLS.Enabled {
		if cfg.GRPC == nil {
			cfg.GRPC = &configgrpc.GRPCServerSettings{
				NetAddr: confignet.NetAddr{
					Endpoint: fmt.Sprintf(":%d", ports.CollectorGRPC),
				},
			}
		}
		cfg.GRPC.TLSSetting = &configtls.TLSServerSetting{
			ClientCAFile: cOpts.TLS.ClientCAPath,
			TLSSetting: configtls.TLSSetting{
				CertFile: cOpts.TLS.CertPath,
				KeyFile:  cOpts.TLS.KeyPath,
			},
		}
	}
	if v.IsSet(collectorApp.CollectorHTTPHostPort) {
		cfg.ThriftHTTP = &confighttp.HTTPServerSettings{
			Endpoint: cOpts.CollectorHTTPHostPort,
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
	if v.IsSet(agentApp.HTTPServerHostPort) {
		if samplingConf == nil {
			samplingConf = &jaegerreceiver.RemoteSamplingConfig{}
		}
		samplingConf.HostEndpoint = v.GetString(agentApp.HTTPServerHostPort)
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
		samplingConf.GRPCClientSettings.TLSSetting.Insecure = !repCfg.TLS.Enabled
		samplingConf.GRPCClientSettings.Endpoint = repCfg.CollectorHostPorts[0]
		samplingConf.GRPCClientSettings.TLSSetting.CAFile = repCfg.TLS.CAPath
		samplingConf.GRPCClientSettings.TLSSetting.CertFile = repCfg.TLS.CertPath
		samplingConf.GRPCClientSettings.TLSSetting.KeyFile = repCfg.TLS.KeyPath
		samplingConf.GRPCClientSettings.TLSSetting.ServerName = repCfg.TLS.ServerName
	}
	return samplingConf
}

// CreateTracesReceiver creates Jaeger receiver trace receiver.
// This function implements OTEL component.ReceiverFactory interface.
func (f *Factory) CreateTracesReceiver(
	ctx context.Context,
	params component.ReceiverCreateParams,
	cfg configmodels.Receiver,
	nextConsumer consumer.TracesConsumer,
) (component.TracesReceiver, error) {
	return f.Wrapped.CreateTracesReceiver(ctx, params, cfg, nextConsumer)
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

// CreateLogsReceiver creates a receiver based on the config.
// If the receiver type does not support logs or if the config is not valid
// error will be returned instead.
func (f Factory) CreateLogsReceiver(
	ctx context.Context,
	params component.ReceiverCreateParams,
	cfg configmodels.Receiver,
	nextConsumer consumer.LogsConsumer,
) (component.LogsReceiver, error) {
	return f.Wrapped.CreateLogsReceiver(ctx, params, cfg, nextConsumer)
}
