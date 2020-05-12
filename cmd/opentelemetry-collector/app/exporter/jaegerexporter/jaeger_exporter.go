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

	"github.com/open-telemetry/opentelemetry-collector/component"
	"github.com/open-telemetry/opentelemetry-collector/config/configmodels"
	"github.com/open-telemetry/opentelemetry-collector/exporter/jaegerexporter"
	"github.com/spf13/viper"

	"github.com/jaegertracing/jaeger/cmd/agent/app/reporter/grpc"
)

// Factory wraps jaegerexporter.Factory and makes the default config configurable via viper.
// For instance this enables using flags as default values in the config object.
type Factory struct {
	// Wrapped is Jaeger receiver.
	Wrapped *jaegerexporter.Factory
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
	repCfg := grpc.ConnBuilder{}
	repCfg.InitFromViper(f.Viper)
	cfg := f.Wrapped.CreateDefaultConfig().(*jaegerexporter.Config)
	if len(repCfg.CollectorHostPorts) > 0 {
		cfg.Endpoint = repCfg.CollectorHostPorts[0]
	}
	cfg.GRPCSettings.TLSConfig.UseSecure = repCfg.TLS.Enabled
	cfg.GRPCSettings.TLSConfig.CaCert = repCfg.TLS.CAPath
	cfg.GRPCSettings.TLSConfig.ClientCert = repCfg.TLS.CertPath
	cfg.GRPCSettings.TLSConfig.ClientKey = repCfg.TLS.KeyPath
	cfg.GRPCSettings.TLSConfig.ServerNameOverride = repCfg.TLS.ServerName
	return cfg
}

// CreateTraceExporter creates Jaeger trace exporter.
// This function implements OTEL component.ExporterFactory interface.
func (f Factory) CreateTraceExporter(
	ctx context.Context,
	params component.ExporterCreateParams,
	cfg configmodels.Exporter,
) (component.TraceExporter, error) {
	return f.Wrapped.CreateTraceExporter(ctx, params, cfg)
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
