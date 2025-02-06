// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

type Factory interface {
	// CreateTraceWriter creates a tracestore.Writer.
	CreateTraceWriter() (tracestore.Writer, error)
}
