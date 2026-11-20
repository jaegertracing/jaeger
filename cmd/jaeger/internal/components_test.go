// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package internal

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/connector"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/pipeline"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/receiver"
)

func TestComponents(t *testing.T) {
	factories, err := Components()

	require.NoError(t, err)

	assert.NotNil(t, factories.Extensions)
	assert.NotNil(t, factories.Receivers)
	assert.NotNil(t, factories.Exporters)
	assert.NotNil(t, factories.Processors)
	assert.NotNil(t, factories.Connectors)

	_, jaegerReceiverFactoryExists := factories.Receivers[component.MustNewType("jaeger")]
	assert.True(t, jaegerReceiverFactoryExists)
}

func TestBatchProcessorConfigs(t *testing.T) {
	factories, err := Components()
	require.NoError(t, err)
	for _, name := range []string{"batch", "queue_batch"} {
		t.Run(name, func(t *testing.T) {
			config := fmt.Sprintf(`yaml:
receivers:
  nop: {}
processors:
  %s: {}
exporters:
  nop: {}
service:
  pipelines:
    traces:
      receivers: [nop]
      processors: [%s]
      exporters: [nop]
`, name, name)
			cfg := loadAndValidateConfig(t, factories, config)
			id := component.MustNewID(name)
			require.Contains(t, cfg.Processors, id)
			assert.Equal(t, factories.Processors[id.Type()].CreateDefaultConfig(), cfg.Processors[id])
			assert.Equal(t, []component.ID{id}, cfg.Service.Pipelines[pipeline.NewID(pipeline.SignalTraces)].Processors)
		})
	}
}

func TestGetOtelcolFactories(t *testing.T) {
	errMock := errors.New("mock error")
	{
		b := defaultBuilders()
		b.extension = mockFactoryMap[extension.Factory](errMock)
		_, err := b.build()
		require.ErrorIs(t, err, errMock)
	}
	{
		b := defaultBuilders()
		b.receiver = mockFactoryMap[receiver.Factory](errMock)
		_, err := b.build()
		require.ErrorIs(t, err, errMock)
	}
	{
		b := defaultBuilders()
		b.exporter = mockFactoryMap[exporter.Factory](errMock)
		_, err := b.build()
		require.ErrorIs(t, err, errMock)
	}
	{
		b := defaultBuilders()
		b.processor = mockFactoryMap[processor.Factory](errMock)
		_, err := b.build()
		require.ErrorIs(t, err, errMock)
	}
	{
		b := defaultBuilders()
		b.connector = mockFactoryMap[connector.Factory](errMock)
		_, err := b.build()
		require.ErrorIs(t, err, errMock)
	}
}

func mockFactoryMap[FACTORY component.Factory](err error) func(factories ...FACTORY) (map[component.Type]FACTORY, error) {
	return func( /* factories */ ...FACTORY) (map[component.Type]FACTORY, error) {
		return nil, err
	}
}
