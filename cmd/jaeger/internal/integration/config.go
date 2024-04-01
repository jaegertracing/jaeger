// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"

	"go.opentelemetry.io/collector/component"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage"
)

type StorageIntegration struct {
	Name       string
	ConfigFile string
}

type storageHost struct {
	t                *testing.T
	storageExtension component.Component
}

func (host storageHost) GetExtensions() map[component.ID]component.Component {
	myMap := make(map[component.ID]component.Component)
	myMap[jaegerstorage.ID] = host.storageExtension
	return myMap
}

func (host storageHost) ReportFatalError(err error) {
	host.t.Fatal(err)
}

func (host storageHost) GetFactory(_ component.Kind, _ component.Type) component.Factory {
	host.t.Log("Calling Factory")
	return nil
}

func (host storageHost) GetExporters() map[component.DataType]map[component.ID]component.Component {
	host.t.Log("Calling Exporters")
	return nil
}
