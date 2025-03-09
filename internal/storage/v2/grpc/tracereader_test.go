// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

func TestTraceReader_GetOperations(t *testing.T) {
	tr := &TraceReader{}

	require.Panics(t, func() {
		_, _ = tr.GetOperations(context.Background(), tracestore.OperationQueryParams{})
	})
}

func TestTraceReader_GetServices(t *testing.T) {
	tr := &TraceReader{}

	require.Panics(t, func() {
		_, _ = tr.GetServices(context.Background())
	})
}

func TestTraceReader_FindTraces(t *testing.T) {
	tr := &TraceReader{}

	require.Panics(t, func() {
		_ = tr.FindTraces(context.Background(), tracestore.TraceQueryParams{})
	})
}

func TestTraceReader_FindTraceIDs(t *testing.T) {
	tr := &TraceReader{}

	require.Panics(t, func() {
		_ = tr.FindTraceIDs(context.Background(), tracestore.TraceQueryParams{})
	})
}
