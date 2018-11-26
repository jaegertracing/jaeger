// Copyright (c) 2017 Uber Technologies, Inc.
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

package storage

import (
	"errors"
	"github.com/jaegertracing/jaeger/pkg/pluginloader"

	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

// Factory defines an interface for a factory that can create implementations of different storage components.
// Implementations are also encouraged to implement plugin.Configurable interface.
//
// See also
//
// plugin.Configurable
type Factory interface {
	// Initialize performs internal initialization of the factory, such as opening connections to the backend store.
	// It is called after all configuration of the factory itself has been done.
	Initialize(metricsFactory metrics.Factory, logger *zap.Logger) error

	// CreateSpanReader creates a spanstore.Reader.
	CreateSpanReader() (spanstore.Reader, error)

	// CreateSpanWriter creates a spanstore.Writer.
	CreateSpanWriter() (spanstore.Writer, error)

	// CreateDependencyReader creates a dependencystore.Reader.
	CreateDependencyReader() (dependencystore.Reader, error)
}

var (
	// ErrArchiveStorageNotConfigured can be returned by the ArchiveFactory when the archive storage is not configured.
	ErrArchiveStorageNotConfigured = errors.New("Archive storage not configured")

	// ErrArchiveStorageNotSupported can be returned by the ArchiveFactory when the archive storage is not supported by the backend.
	ErrArchiveStorageNotSupported = errors.New("Archive storage not supported")
)

// ArchiveFactory is an additional interface that can be implemented by a factory to support trace archiving.
type ArchiveFactory interface {
	// CreateArchiveSpanReader creates a spanstore.Reader.
	CreateArchiveSpanReader() (spanstore.Reader, error)

	// CreateArchiveSpanWriter creates a spanstore.Writer.
	CreateArchiveSpanWriter() (spanstore.Writer, error)
}


// PluginFactory is an additional interface that can be implemented by a factory to support plugins.
type PluginFactory interface {
	// InitializePlugin performs internal initialization of the plugins
	// It is called after all configuration and initialization of the factory itself has been done
	InitializePlugin(pluginLoader pluginloader.PluginLoader) error
}
