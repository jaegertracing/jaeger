// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"

	"go.opentelemetry.io/collector/component"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/exporters/storageexporter"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage"
	"github.com/jaegertracing/jaeger/plugin/storage/integration"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

type StorageIntegration struct {
	Name              string
	ConfigFile        string
	SpanWriter        spanstore.Writer
	SpanReader        spanstore.Reader
	ArchiveSpanWriter spanstore.Writer
	ArchiveSpanReader spanstore.Reader
	storageExtension  component.Component
	exporterCfg       *storageexporter.Config
	tests             *integration.StorageIntegration
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
	return nil
}

func (host storageHost) GetExporters() map[component.DataType]map[component.ID]component.Component {
	return nil
}
