// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package shared

import (
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

// StoragePlugin is the interface we're exposing as a plugin.
type StoragePlugin interface {
	SpanReader() spanstore.Reader
	SpanWriter() spanstore.Writer
	DependencyReader() dependencystore.Reader
}

// StreamingSpanWriterPlugin is the interface we're exposing as a plugin.
type StreamingSpanWriterPlugin interface {
	StreamingSpanWriter() spanstore.Writer
}

// PluginCapabilities allow expose plugin its capabilities.
type PluginCapabilities interface {
	Capabilities() (*Capabilities, error)
}

// Capabilities contains information about plugin capabilities
type Capabilities struct {
	ArchiveSpanReader   bool
	ArchiveSpanWriter   bool
	StreamingSpanWriter bool
}

// PluginServices defines services plugin can expose
type PluginServices struct {
	Store               StoragePlugin
	StreamingSpanWriter StreamingSpanWriterPlugin
}
