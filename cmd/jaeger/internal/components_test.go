// Copyright (c) 2024 The Jaeger Authors.
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

package internal

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/connector"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/receiver"
)

func TestComponents(t *testing.T) {
	factories, err := components()

	require.NoError(t, err)

	assert.NotNil(t, factories.Extensions)
	assert.NotNil(t, factories.Receivers)
	assert.NotNil(t, factories.Exporters)
	assert.NotNil(t, factories.Processors)
	assert.NotNil(t, factories.Connectors)

	_, jaegerReceiverFactoryExists := factories.Receivers["jaeger"]
	assert.True(t, jaegerReceiverFactoryExists)
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
	return func(factories ...FACTORY) (map[component.Type]FACTORY, error) {
		return nil, err
	}
}
