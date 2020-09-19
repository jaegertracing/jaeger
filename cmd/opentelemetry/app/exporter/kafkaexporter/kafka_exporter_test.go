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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configcheck"
	otelKafkaExporter "go.opentelemetry.io/collector/exporter/kafkaexporter"

	jConfig "github.com/jaegertracing/jaeger/pkg/config"
)

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
