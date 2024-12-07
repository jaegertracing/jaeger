// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"github.com/jaegertracing/jaeger/storage_v2"
)

// Factory defines an interface for a factory that can create implementations of
// different span storage components.
type Factory interface {
	storage_v2.FactoryBase

	// CreateTraceReader creates a spanstore.Reader.
	CreateTraceReader() (Reader, error)

	// CreateTraceWriter creates a spanstore.Writer.
	CreateTraceWriter() (Writer, error)
}
