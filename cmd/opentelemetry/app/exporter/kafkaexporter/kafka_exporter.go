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

package kafkaexporter

import (
	"context"

	"github.com/spf13/viper"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configmodels"
	"go.opentelemetry.io/collector/config/configtls"
	"go.opentelemetry.io/collector/exporter/kafkaexporter"

	"github.com/jaegertracing/jaeger/plugin/storage/kafka"
)

// TypeStr defines exporter type.
const TypeStr = "kafka"

// Factory wraps kafkaexporter.Factory and makes the default config configurable via viper.
// For instance this enables using flags as default values in the config object.
type Factory struct {
	// Wrapped is kafka exporter
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
	cfg := f.Wrapped.CreateDefaultConfig().(*kafkaexporter.Config)

	// InitFromViper fails if certain fields are not set.  Setting them here
	//  to prevent the process from exiting.
	f.Viper.Set("kafka.producer.required-acks", "local")
	f.Viper.Set("kafka.producer.compression", "none")

	opts := &kafka.Options{}
	opts.InitFromViper(f.Viper)

	cfg.Encoding = mustOtelEncodingForJaegerEncoding(opts.Encoding)
	cfg.Topic = opts.Topic
	cfg.Brokers = opts.Config.Brokers
	cfg.ProtocolVersion = opts.Config.ProtocolVersion

	if opts.Config.Authentication == "kerberos" {
		cfg.Authentication.Kerberos = &kafkaexporter.KerberosConfig{
			ServiceName: opts.Config.Kerberos.ServiceName,
			Realm:       opts.Config.Kerberos.Realm,
			UseKeyTab:   opts.Config.Kerberos.UseKeyTab,
			Username:    opts.Config.Kerberos.Username,
			Password:    opts.Config.Kerberos.Password,
			ConfigPath:  opts.Config.Kerberos.ConfigPath,
			KeyTabPath:  opts.Config.Kerberos.KeyTabPath,
		}
	}

	if opts.Config.Authentication == "plaintext" {
		cfg.Authentication.PlainText = &kafkaexporter.PlainTextConfig{
			Username: opts.Config.PlainText.UserName,
			Password: opts.Config.PlainText.Password,
		}
	}

	if opts.Config.Authentication == "tls" && opts.Config.TLS.Enabled {
		cfg.Authentication.TLS = &configtls.TLSClientSetting{
			TLSSetting: configtls.TLSSetting{
				CAFile:   opts.Config.TLS.CAPath,
				CertFile: opts.Config.TLS.CertPath,
				KeyFile:  opts.Config.TLS.KeyPath,
			},
			ServerName: opts.Config.TLS.ServerName,
			Insecure:   opts.Config.TLS.SkipHostVerify,
		}
	}

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

// mustOtelEncodingForJaegerEncoding translates a jaeger encoding to a otel encoding
func mustOtelEncodingForJaegerEncoding(jaegerEncoding string) string {
	switch jaegerEncoding {
	case kafka.EncodingProto:
		return "jaeger_proto"
	case kafka.EncodingJSON:
		return "jaeger_json"
	case encodingOTLPProto:
		return "otlp_proto"
	}

	panic(jaegerEncoding + " is not a supported kafka encoding in the OTEL collector.")
}
