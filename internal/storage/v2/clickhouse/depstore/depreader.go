// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package depstore

import (
	"context"

	"github.com/jaegertracing/jaeger-idl/model/v1"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
	clickhouseTracestore "github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore"
)

// Reader wraps the ClickHouse tracestore.Reader and implements depstore.Reader.
type Reader struct {
	traceReader *clickhouseTracestore.Reader
}

// NewReader returns a new ClickHouse dependency reader that wraps the trace reader.
func NewReader(traceReader *clickhouseTracestore.Reader) *Reader {
	return &Reader{
		traceReader: traceReader,
	}
}

// GetDependencies fetches service dependency links from ClickHouse by delegating
// to the traceReader's GetDependencies function.
func (r *Reader) GetDependencies(
	ctx context.Context,
	params depstore.QueryParameters,
) ([]model.DependencyLink, error) {
	return r.traceReader.GetDependencies(ctx, params)
}
