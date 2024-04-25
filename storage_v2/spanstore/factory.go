// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package spanstore

// Factory defines an interface for a factory that can create implementations of different storage components.
// Implementations are also encouraged to implement plugin.Configurable interface.
type Factory interface {
	// Initialize performs internal initialization of the factory, such as opening connections to the backend store.
	// It is called after all configuration of the factory itself has been done.
	Initialize() error

	// CreateSpanReader creates a spanstore.Reader.
	CreateTraceReader() (Reader, error)

	// CreateSpanWriter creates a spanstore.Writer.
	CreateTraceWriter() (Writer, error)
}
