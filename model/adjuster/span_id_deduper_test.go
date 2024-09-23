// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"

	"github.com/jaegertracing/jaeger/model"
)

func newDuplicatedSpansTrace() *model.Trace {
	traceID := model.NewTraceID(0, 42)
	return &model.Trace{
		Spans: []*model.Span{
			{
				// client span
				TraceID: traceID,
				SpanID:  clientSpanID,
				Tags: model.KeyValues{
					// span.kind = client
					model.String(keySpanKind, trace.SpanKindClient.String()),
				},
			},
			{
				// server span
				TraceID: traceID,
				SpanID:  clientSpanID, // shared span ID
				Tags: model.KeyValues{
					// span.kind = server
					model.String(keySpanKind, trace.SpanKindServer.String()),
				},
			},
			{
				// some other span, child of server span
				TraceID:    traceID,
				SpanID:     anotherSpanID,
				References: []model.SpanRef{model.NewChildOfRef(traceID, clientSpanID)},
			},
		},
	}
}

func TestDedupeBySpanID(t *testing.T) {
	trace := newZipkinTrace()
	deduper := DedupeBySpanID()
	trace, err := deduper.Adjust(trace)
	require.NoError(t, err)

	assert.Len(t, trace.Spans, 2, "should dedupe spans")
	assert.Equal(t, clientSpanID, trace.Spans[0].SpanID, "client span should be kept")
	assert.Equal(t, anotherSpanID, trace.Spans[1].SpanID, "3rd span should be kept")
}
