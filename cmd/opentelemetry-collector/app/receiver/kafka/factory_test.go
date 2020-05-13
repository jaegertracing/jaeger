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

package kafka

import (
	"context"
	"testing"

	"github.com/open-telemetry/opentelemetry-collector/component"
	"github.com/open-telemetry/opentelemetry-collector/config/configcheck"
	"github.com/open-telemetry/opentelemetry-collector/config/configerror"
	"github.com/open-telemetry/opentelemetry-collector/config/configmodels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	ingesterApp "github.com/jaegertracing/jaeger/cmd/ingester/app"
	jConfig "github.com/jaegertracing/jaeger/pkg/config"
)

func TestCreateTraceReceiver(t *testing.T) {
	v, _ := jConfig.Viperize(ingesterApp.AddFlags)
	opts := DefaultOptions()
	opts.InitFromViper(v)
	factory := &Factory{OptionsFactory: func() *ingesterApp.Options {
		return opts
	}}
	exporter, err := factory.CreateTraceReceiver(context.Background(), component.ReceiverCreateParams{Logger: zap.NewNop()}, factory.CreateDefaultConfig(), nil)
	require.Nil(t, exporter)
	assert.Contains(t, err.Error(), "kafka: client has run out of available brokers to talk to (Is your cluster reachable?)")
}

func TestCreateTraceExporter_nilConfig(t *testing.T) {
	factory := &Factory{}
	exporter, err := factory.CreateTraceReceiver(context.Background(), component.ReceiverCreateParams{}, nil, nil)
	require.Nil(t, exporter)
	assert.Contains(t, err.Error(), "could not cast configuration to jaeger_kafka")
}

func TestCreateMetricsExporter(t *testing.T) {
	f := Factory{OptionsFactory: DefaultOptions}
	mReceiver, err := f.CreateMetricsReceiver(context.Background(), component.ReceiverCreateParams{}, f.CreateDefaultConfig(), nil)
	assert.Equal(t, err, configerror.ErrDataTypeIsNotSupported)
	assert.Nil(t, mReceiver)
}

func TestCreateDefaultConfig(t *testing.T) {
	factory := Factory{OptionsFactory: DefaultOptions}
	cfg := factory.CreateDefaultConfig()
	assert.NotNil(t, cfg, "failed to create default config")
	assert.NoError(t, configcheck.ValidateConfig(cfg))
}

func TestType(t *testing.T) {
	factory := Factory{OptionsFactory: DefaultOptions}
	assert.Equal(t, configmodels.Type(TypeStr), factory.Type())
}
