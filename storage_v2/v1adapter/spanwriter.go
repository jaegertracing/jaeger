// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package v1adapter

import (
	"context"

	jaegerTranslator "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/jaeger"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v1/spanstore"
	"github.com/jaegertracing/jaeger/storage_v2/tracestore"
)

var _ spanstore.Writer = (*SpanWriter)(nil)

// SpanReader wraps a tracestore.Writer so that it can be downgraded to implement
// the v1 spanstore.Writer interface.
type SpanWriter struct {
	traceWriter tracestore.Writer
}

func NewSpanWriter(traceWriter tracestore.Writer) *SpanWriter {
	return &SpanWriter{
		traceWriter: traceWriter,
	}
}

func (sw *SpanWriter) WriteSpan(ctx context.Context, span *model.Span) error {
	traces, _ := jaegerTranslator.ProtoToTraces([]*model.Batch{{Spans: []*model.Span{span}}})
	return sw.traceWriter.WriteTraces(ctx, traces)
}
