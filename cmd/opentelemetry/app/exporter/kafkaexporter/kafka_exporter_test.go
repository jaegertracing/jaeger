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
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/configcheck"
	"go.opentelemetry.io/collector/config/configmodels"
	"go.opentelemetry.io/collector/config/configtest"
	"go.opentelemetry.io/collector/config/configtls"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	otelKafkaExporter "go.opentelemetry.io/collector/exporter/kafkaexporter"

	"github.com/jaegertracing/jaeger/cmd/flags"
	jConfig "github.com/jaegertracing/jaeger/pkg/config"
)

func TestType(t *testing.T) {
	factory := &Factory{
		Wrapped: otelKafkaExporter.NewFactory(),
	}

	assert.Equal(t, configmodels.Type(TypeStr), factory.Type())
}

func TestDefaultConfig(t *testing.T) {
	v, c := jConfig.Viperize(AddFlags)
	err := c.ParseFlags([]string{""})
	require.NoError(t, err)

	factory := &Factory{
		Wrapped: otelKafkaExporter.NewFactory(),
		Viper:   v,
	}
	defaultCfg := factory.CreateDefaultConfig().(*otelKafkaExporter.Config)

	assert.NoError(t, configcheck.ValidateConfig(defaultCfg))
	assert.Equal(t, "jaeger-spans", defaultCfg.Topic)
	assert.Equal(t, "jaeger_proto", defaultCfg.Encoding)
	assert.Equal(t, "", defaultCfg.ProtocolVersion)
	assert.Equal(t, []string{"127.0.0.1:9092"}, defaultCfg.Brokers)
	assert.Nil(t, defaultCfg.Authentication.Kerberos)
	assert.Nil(t, defaultCfg.Authentication.TLS)
	assert.Nil(t, defaultCfg.Authentication.PlainText)
}

func TestLoadConfigAndFlags(t *testing.T) {
	metadataSettings := otelKafkaExporter.Metadata{
		Full: true,
		Retry: otelKafkaExporter.MetadataRetry{
			Max:     3,
			Backoff: 250 * time.Millisecond,
		},
	}
	queueSettings := exporterhelper.CreateDefaultQueueSettings()
	queueSettings.Enabled = false // disabled by default in upstream
	exporterSettings := configmodels.ExporterSettings{
		TypeVal: "kafka",
		NameVal: "kafka",
	}

	tests := []struct {
		name        string
		jFlags      []string
		oConfigFile string
		expected    *otelKafkaExporter.Config
	}{
		{
			name:        "jaeger-kafka-auth",
			jFlags:      []string{"--config-file=./testdata/jaeger-config.yaml", "--kafka.producer.topic=jaeger-test", "--kafka.producer.brokers=host1,host2"},
			oConfigFile: filepath.Join(".", "testdata", "config.yaml"),
			expected: &otelKafkaExporter.Config{
				Topic:            "jaeger-prod",
				Encoding:         "emojis",
				Brokers:          []string{"foo", "bar"},
				Metadata:         metadataSettings,
				ExporterSettings: exporterSettings,
				TimeoutSettings:  exporterhelper.CreateDefaultTimeoutSettings(),
				QueueSettings:    queueSettings,
				RetrySettings:    exporterhelper.CreateDefaultRetrySettings(),
				Authentication: otelKafkaExporter.Authentication{
					PlainText: &otelKafkaExporter.PlainTextConfig{
						Username: "user",
					},
					Kerberos: &otelKafkaExporter.KerberosConfig{
						ServiceName: "kafka",
						Username:    "from-jaeger-config",
						Realm:       "jaeger",
						ConfigPath:  "/etc/foo",
						KeyTabPath:  "/etc/security/kafka.keytab",
					},
					TLS: &configtls.TLSClientSetting{
						Insecure: true,
						TLSSetting: configtls.TLSSetting{
							CAFile:   "ca.crt",
							KeyFile:  "key.crt",
							CertFile: "",
						},
					},
				},
			},
		},
		{
			name:        "jaeger-tls-auth",
			jFlags:      []string{"--kafka.producer.authentication=tls", "--kafka.producer.tls.cert=from-jaeger-flag"},
			oConfigFile: filepath.Join(".", "testdata", "config.yaml"),
			expected: &otelKafkaExporter.Config{
				Topic:            "jaeger-prod",
				Encoding:         "emojis",
				Brokers:          []string{"foo", "bar"},
				Metadata:         metadataSettings,
				ExporterSettings: exporterSettings,
				TimeoutSettings:  exporterhelper.CreateDefaultTimeoutSettings(),
				QueueSettings:    queueSettings,
				RetrySettings:    exporterhelper.CreateDefaultRetrySettings(),
				Authentication: otelKafkaExporter.Authentication{
					PlainText: &otelKafkaExporter.PlainTextConfig{
						Username: "user",
					},
					Kerberos: &otelKafkaExporter.KerberosConfig{
						ServiceName: "",
						Username:    "",
						Realm:       "jaeger",
						ConfigPath:  "/etc/foo",
						KeyTabPath:  "",
					},
					TLS: &configtls.TLSClientSetting{
						Insecure: true,
						TLSSetting: configtls.TLSSetting{
							CAFile:   "ca.crt",
							KeyFile:  "key.crt",
							CertFile: "from-jaeger-flag",
						},
					},
				},
			},
		},
		{
			name:        "jaeger-plaintext-auth",
			jFlags:      []string{"--kafka.producer.authentication=plaintext", "--kafka.producer.plaintext.password=from-jaeger-flag"},
			oConfigFile: filepath.Join(".", "testdata", "config.yaml"),
			expected: &otelKafkaExporter.Config{
				Topic:            "jaeger-prod",
				Encoding:         "emojis",
				Brokers:          []string{"foo", "bar"},
				Metadata:         metadataSettings,
				ExporterSettings: exporterSettings,
				TimeoutSettings:  exporterhelper.CreateDefaultTimeoutSettings(),
				QueueSettings:    queueSettings,
				RetrySettings:    exporterhelper.CreateDefaultRetrySettings(),
				Authentication: otelKafkaExporter.Authentication{
					PlainText: &otelKafkaExporter.PlainTextConfig{
						Username: "user",
						Password: "from-jaeger-flag",
					},
					Kerberos: &otelKafkaExporter.KerberosConfig{
						ServiceName: "",
						Username:    "",
						Realm:       "jaeger",
						ConfigPath:  "/etc/foo",
						KeyTabPath:  "",
					},
					TLS: &configtls.TLSClientSetting{
						Insecure: true,
						TLSSetting: configtls.TLSSetting{
							CAFile:   "ca.crt",
							KeyFile:  "key.crt",
							CertFile: "",
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, c := jConfig.Viperize(AddFlags, flags.AddConfigFileFlag)
			err := c.ParseFlags(tt.jFlags)
			require.NoError(t, err)

			err = flags.TryLoadConfigFile(v)
			require.NoError(t, err)

			factory := &Factory{
				Wrapped: otelKafkaExporter.NewFactory(),
				Viper:   v,
			}

			factories, err := componenttest.ExampleComponents()
			factories.Exporters[TypeStr] = factory
			cfg, err := configtest.LoadConfigFile(t, tt.oConfigFile, factories)
			require.NoError(t, err)
			require.NotNil(t, cfg)

			kafkaCfg := cfg.Exporters[TypeStr].(*otelKafkaExporter.Config)
			require.Equal(t, tt.expected, kafkaCfg)
		})
	}
}

func TestFactoryPassthrough(t *testing.T) {
	factory := &Factory{
		Wrapped: &componenttest.ExampleExporterFactory{},
	}

	actualLogs, errA := factory.CreateLogsExporter(context.Background(), component.ExporterCreateParams{}, nil)
	expectedLogs, errB := factory.Wrapped.CreateLogsExporter(context.Background(), component.ExporterCreateParams{}, nil)
	assert.Equal(t, actualLogs, expectedLogs)
	assert.Equal(t, errA, errB)

	actualTrace, errA := factory.CreateTraceExporter(context.Background(), component.ExporterCreateParams{}, nil)
	expectedTrace, errB := factory.Wrapped.CreateTraceExporter(context.Background(), component.ExporterCreateParams{}, nil)
	assert.Equal(t, actualTrace, expectedTrace)
	assert.Equal(t, errA, errB)

	actualMetrics, errA := factory.CreateMetricsExporter(context.Background(), component.ExporterCreateParams{}, nil)
	expectedMetrics, errB := factory.Wrapped.CreateMetricsExporter(context.Background(), component.ExporterCreateParams{}, nil)
	assert.Equal(t, actualMetrics, expectedMetrics)
	assert.Equal(t, errA, errB)
}
