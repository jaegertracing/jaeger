// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"fmt"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
)

type truncatingWriter struct {
	next                   Writer
	MaxAttributeValueBytes int
}

func NewTruncatingWriter(next Writer, maxBytes int) Writer {
	if maxBytes <= 0 {
		return next
	}

	return &truncatingWriter{
		next:                   next,
		MaxAttributeValueBytes: maxBytes,
	}
}

func (w *truncatingWriter) WriteTraces(ctx context.Context, td ptrace.Traces) error {
	tracesToWrite := td
	if td.IsReadOnly() {
		tracesToWrite = ptrace.NewTraces()
		td.CopyTo(tracesToWrite)
	}
	w.truncateTraces(td)
	return w.next.WriteTraces(ctx, td)
}

func (w *truncatingWriter) truncateTraces(td ptrace.Traces) {
	resourceSpans := td.ResourceSpans()
	for i := 0; i < resourceSpans.Len(); i++ {
		rs := resourceSpans.At(i)
		w.truncateMap(rs.Resource().Attributes())

		scopeSpans := rs.ScopeSpans()

		for j := 0; j < scopeSpans.Len(); j++ {
			ss := scopeSpans.At(j)
			w.truncateMap(ss.Scope().Attributes())

			spans := ss.Spans()
			for k := 0; k < spans.Len(); k++ {
				span := spans.At(k)

				w.truncateMap(span.Attributes())

				events := span.Events()
				for l := 0; l < events.Len(); l++ {
					w.truncateMap(events.At(l).Attributes())
				}

				links := span.Links()
				for l := 0; l < links.Len(); l++ {
					w.truncateMap(links.At(l).Attributes())
				}
			}
		}
	}
}

func (w *truncatingWriter) truncateMap(attrs pcommon.Map) {
	attrs.Range(func(k string, v pcommon.Value) bool {
		if v.Type() == pcommon.ValueTypeStr {
			strVal := v.AsString()
			if len(strVal) > w.MaxAttributeValueBytes {
				truncated := strVal[:w.MaxAttributeValueBytes]
				suffix := fmt.Sprintf(" [truncated: %d bytes]", len(strVal))
				attrs.PutStr(k, truncated+suffix)
			}
		}
		return true
	})
}

type truncatingFactory struct {
	Factory
	MaxAttributeValueBytes int
}

func (f *truncatingFactory) CreateTraceWriter() (Writer, error) {
	writer, err := f.Factory.CreateTraceWriter()
	if err != nil {
		return nil, err
	}
	return NewTruncatingWriter(writer, f.MaxAttributeValueBytes), nil
}

type truncatingDepFactory struct {
	*truncatingFactory
	depstore.Factory
}

func NewTruncatingFactory(base Factory, maxBytes int) Factory {
	if maxBytes <= 0 {
		return base
	}

	tf := &truncatingFactory{
		Factory:                base,
		MaxAttributeValueBytes: maxBytes,
	}

	if depBase, ok := base.(depstore.Factory); ok {
		return &truncatingDepFactory{
			truncatingFactory: tf,
			Factory:           depBase,
		}
	}

	return tf
}
