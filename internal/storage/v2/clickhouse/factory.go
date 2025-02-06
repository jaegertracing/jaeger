// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhouse

import "github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"

// Factory implements storage.Factory and creates write-only storage components backed by clickhouse.
type Factory struct{}

// NewFactory creates a new Factory.
func NewFactory() *Factory {
	return &Factory{}
}

func (*Factory) CreateTraceWriter() (tracestore.Writer, error) {
	return nil, nil
}
