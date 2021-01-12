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

package kafkareceiver

import (
	"context"

	"github.com/Shopify/sarama"
	"github.com/spf13/viper"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configmodels"
	"go.opentelemetry.io/collector/config/configtls"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/exporter/kafkaexporter"
	"go.opentelemetry.io/collector/receiver/kafkareceiver"

	ingesterApp "github.com/jaegertracing/jaeger/cmd/ingester/app"
	"github.com/jaegertracing/jaeger/plugin/storage/kafka"
)

// TypeStr defines receiver type.
const TypeStr = "kafka"

// Factory wraps kafkareceiver.Factory and makes the default config configurable via viper.
// For instance this enables using flags as default values in the config object.
type Factory struct {
	// Wrapped is Kafka receiver.
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
	cfg := f.Wrapped.CreateDefaultConfig().(*kafkareceiver.Config)
	// load jaeger config
	opts := &ingesterApp.Options{}
	opts.InitFromViper(f.Viper)

	cfg.Brokers = opts.Brokers
	cfg.ClientID = opts.ClientID
	cfg.Encoding = mustOtelEncodingForJaegerEncoding(opts.Encoding)
	cfg.GroupID = opts.GroupID
	cfg.Topic = opts.Topic
	cfg.ProtocolVersion = opts.ProtocolVersion

	// kafka consumer groups require a min version of V0_10_2_0.  if no version is specified
	//  we will assume this
	if len(cfg.ProtocolVersion) == 0 {
		cfg.ProtocolVersion = sarama.V0_10_2_0.String()
	}

	if opts.Authentication == "kerberos" {
		cfg.Authentication.Kerberos = &kafkaexporter.KerberosConfig{
			ServiceName: opts.Kerberos.ServiceName,
			Realm:       opts.Kerberos.Realm,
			UseKeyTab:   opts.Kerberos.UseKeyTab,
			Username:    opts.Kerberos.Username,
			Password:    opts.Kerberos.Password,
			ConfigPath:  opts.Kerberos.ConfigPath,
			KeyTabPath:  opts.Kerberos.KeyTabPath,
		}
	}

	if opts.Authentication == "plaintext" {
		cfg.Authentication.PlainText = &kafkaexporter.PlainTextConfig{
			Username: opts.PlainText.UserName,
			Password: opts.PlainText.Password,
		}
	}

	if opts.Authentication == "tls" && opts.TLS.Enabled {
		cfg.Authentication.TLS = &configtls.TLSClientSetting{
			TLSSetting: configtls.TLSSetting{
				CAFile:   opts.TLS.CAPath,
				CertFile: opts.TLS.CertPath,
				KeyFile:  opts.TLS.KeyPath,
			},
			ServerName: opts.TLS.ServerName,
			Insecure:   opts.TLS.SkipHostVerify,
		}
	}

	return cfg
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

// mustOtelEncodingForJaegerEncoding translates a jaeger encoding to a otel encoding
func mustOtelEncodingForJaegerEncoding(jaegerEncoding string) string {
	switch jaegerEncoding {
	case kafka.EncodingProto:
		return "jaeger_proto"
	case kafka.EncodingJSON:
		return "jaeger_json"
	case encodingOTLPProto:
		return "otlp_proto"
	case encodingZipkinProto:
		return "zipkin_proto"
	case encodingZipkinJSON:
		return "zipkin_json"
	case kafka.EncodingZipkinThrift:
		return "zipkin_thrift"
	}

	panic(jaegerEncoding + " is not a supported kafka encoding in the OTEL collector.")
}
