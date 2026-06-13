// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

// Factory defines an interface for a factory that can create implementations of
// different span storage components.
type Factory interface {
	// CreateTraceReader creates a spanstore.Reader.
	CreateTraceReader() (Reader, error)

	// CreateTraceWriter creates a spanstore.Writer.
	CreateTraceWriter() (Writer, error)
}
