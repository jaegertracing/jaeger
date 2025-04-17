// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package v1adapter

import (
	"context"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

var _ spanstore.Writer = (*SpanWriter)(nil)

// SpanReader wraps a tracestore.Writer so that it can be downgraded to implement
// the v1 spanstore.Writer interface.
type SpanWriter struct {
	traceWriter tracestore.Writer
}

func (sw *SpanWriter) WriteSpan(ctx context.Context, span *model.Span) error {
	traces := V1BatchesToTraces([]*model.Batch{{Spans: []*model.Span{span}}})
	return sw.traceWriter.WriteTraces(ctx, traces)
}
