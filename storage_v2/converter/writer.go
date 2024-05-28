// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package converter

import (
	"context"
	"fmt"

	jaeger2otlp "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/jaeger"

	"github.com/jaegertracing/jaeger/model"
	spanstore_v1 "github.com/jaegertracing/jaeger/storage/spanstore"
	"github.com/jaegertracing/jaeger/storage_v2/spanstore"
)

type SpanWriter struct {
	traceWriter spanstore.Writer
}

func NewSpanWriter(traceWriter spanstore.Writer) (spanstore_v1.Writer, error) {
	return &SpanWriter{
		traceWriter: traceWriter,
	}, nil
}

// WriteSpan implements spanstore.Writer.
func (s *SpanWriter) WriteSpan(ctx context.Context, span *model.Span) error {
	batch := []*model.Batch{{Spans: []*model.Span{span}}}
	td, err := jaeger2otlp.ProtoToTraces(batch)
	if err != nil {
		return fmt.Errorf("cannot transform Jaeger span to OTLP trace format: %w", err)
	}
	return s.traceWriter.WriteTraces(ctx, td)
}
