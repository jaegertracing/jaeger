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

	"github.com/spf13/viper"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configmodels"
	"go.opentelemetry.io/collector/exporter/jaegerexporter"

	grpcRep "github.com/jaegertracing/jaeger/cmd/agent/app/reporter/grpc"
)

// Factory wraps jaegerexporter.Factory and makes the default config configurable via viper.
// For instance this enables using flags as default values in the config object.
type Factory struct {
	// Wrapped is Jaeger receiver.
	Wrapped component.ExporterFactory
	// Viper is used to get configuration values for default configuration
	Viper *viper.Viper
}

var _ component.ExporterFactory = (*Factory)(nil)

// Type returns the type of the exporter.
func (f Factory) Type() configmodels.Type {
	return f.Wrapped.Type()
}

// CreateDefaultConfig returns default configuration of Factory.
// This function implements OTEL component.ExporterFactoryBase interface.
func (f Factory) CreateDefaultConfig() configmodels.Exporter {
	repCfg := grpcRep.ConnBuilder{}
	repCfg.InitFromViper(f.Viper)
	cfg := f.Wrapped.CreateDefaultConfig().(*jaegerexporter.Config)
	if len(repCfg.CollectorHostPorts) > 0 {
		cfg.Endpoint = repCfg.CollectorHostPorts[0]
	}
	cfg.GRPCClientSettings.TLSSetting.Insecure = !repCfg.TLS.Enabled
	cfg.GRPCClientSettings.TLSSetting.CAFile = repCfg.TLS.CAPath
	cfg.GRPCClientSettings.TLSSetting.CertFile = repCfg.TLS.CertPath
	cfg.GRPCClientSettings.TLSSetting.KeyFile = repCfg.TLS.KeyPath
	cfg.GRPCClientSettings.TLSSetting.ServerName = repCfg.TLS.ServerName
	return cfg
}

// CreateTracesExporter creates Jaeger trace exporter.
// This function implements OTEL component.ExporterFactory interface.
func (f Factory) CreateTracesExporter(
	ctx context.Context,
	params component.ExporterCreateParams,
	cfg configmodels.Exporter,
) (component.TracesExporter, error) {
	return f.Wrapped.CreateTracesExporter(ctx, params, cfg)
}

// CreateMetricsExporter creates a metrics exporter based on provided config.
// This function implements component.ExporterFactory.
func (f Factory) CreateMetricsExporter(
	ctx context.Context,
	params component.ExporterCreateParams,
	cfg configmodels.Exporter,
) (component.MetricsExporter, error) {
	return f.Wrapped.CreateMetricsExporter(ctx, params, cfg)
}

// CreateLogsExporter creates a metrics exporter based on provided config.
// This function implements component.ExporterFactory.
func (f Factory) CreateLogsExporter(
	ctx context.Context,
	params component.ExporterCreateParams,
	cfg configmodels.Exporter,
) (component.LogsExporter, error) {
	return f.Wrapped.CreateLogsExporter(ctx, params, cfg)
}
