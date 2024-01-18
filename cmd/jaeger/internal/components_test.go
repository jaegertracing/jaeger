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
	_, err := getOtelcolFactories(
		mockExtension(errors.New("mock error")),
		mockReceiver(nil),
		mockExporter(nil),
		mockProcessor(nil),
		mockConnector(nil),
	)
	require.Error(t, err)

	_, err = getOtelcolFactories(
		mockExtension(nil),
		mockReceiver(errors.New("mockReceiver error")),
		mockExporter(nil),
		mockProcessor(nil),
		mockConnector(nil),
	)
	require.Error(t, err)

	_, err = getOtelcolFactories(
		mockExtension(nil),
		mockReceiver(nil),
		mockExporter(errors.New("mockExporter error")),
		mockProcessor(nil),
		mockConnector(nil),
	)
	require.Error(t, err)

	_, err = getOtelcolFactories(
		mockExtension(nil),
		mockReceiver(nil),
		mockExporter(nil),
		mockProcessor(errors.New("mockProcessor error")),
		mockConnector(nil),
	)
	require.Error(t, err)

	_, err = getOtelcolFactories(
		mockExtension(nil),
		mockReceiver(nil),
		mockExporter(nil),
		mockProcessor(nil),
		mockConnector(errors.New("mockConnector error")),
	)
	require.Error(t, err)

}

func mockExtension(err error) func(factories ...extension.Factory) (map[component.Type]extension.Factory, error) {
	return func(factories ...extension.Factory) (map[component.Type]extension.Factory, error) {
		return nil, err
	}
}
func mockReceiver(err error) func(factories ...receiver.Factory) (map[component.Type]receiver.Factory, error) {
	return func(factories ...receiver.Factory) (map[component.Type]receiver.Factory, error) {
		return nil, err
	}
}
func mockExporter(err error) func(factories ...exporter.Factory) (map[component.Type]exporter.Factory, error) {
	return func(factories ...exporter.Factory) (map[component.Type]exporter.Factory, error) {
		return nil, err
	}
}
func mockConnector(err error) func(factories ...connector.Factory) (map[component.Type]connector.Factory, error) {
	return func(factories ...connector.Factory) (map[component.Type]connector.Factory, error) {
		return nil, err
	}
}
func mockProcessor(err error) func(factories ...processor.Factory) (map[component.Type]processor.Factory, error) {
	return func(factories ...processor.Factory) (map[component.Type]processor.Factory, error) {
		return nil, err
	}
}
