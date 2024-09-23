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
				TraceID: traceID,
				SpanID:  clientSpanID,
				Tags: model.KeyValues{
					// span.kind = server
					model.String(keySpanKind, trace.SpanKindServer.String()),
				},
			},
			{
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

func newUniqueSpansTrace() *model.Trace {
	traceID := model.NewTraceID(0, 42)
	return &model.Trace{
		Spans: []*model.Span{
			{
				TraceID: traceID,
				SpanID:  clientSpanID,
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

func TestDedupeBySpanIDTriggers(t *testing.T) {
	trace := newDuplicatedSpansTrace()
	deduper := DedupeBySpanID()
	trace, err := deduper.Adjust(trace)
	require.NoError(t, err)

	assert.Len(t, trace.Spans, 2, "should dedupe spans")
	assert.Equal(t, clientSpanID, trace.Spans[0].SpanID, "client span should be kept")
	assert.Equal(t, anotherSpanID, trace.Spans[1].SpanID, "3rd span should be kept")
}

func TestDedupeBySpanIDNotTriggered(t *testing.T) {
	trace := newUniqueSpansTrace()
	deduper := DedupeBySpanID()
	trace, err := deduper.Adjust(trace)
	require.NoError(t, err)

	assert.Len(t, trace.Spans, 2, "should not dedupe spans")
	var ids [2]int
	for i, span := range trace.Spans {
		ids[i] = int(span.SpanID)
	}
	assert.ElementsMatch(t, []int{int(clientSpanID), int(anotherSpanID)}, ids, "should keep unique span IDs")
}

func TestDedupeBySpanIDEmpty(t *testing.T) {
	trace := &model.Trace{}
	deduper := DedupeBySpanID()
	trace, err := deduper.Adjust(trace)
	require.NoError(t, err)

	assert.Empty(t, trace.Spans, "should be empty")
}

func TestDedupeBySpanIDManyManySpans(t *testing.T) {
	traceID := model.NewTraceID(0, 42)
	spans := make([]*model.Span, 0, 100)
	const distinctSpanIDs = 10
	for i := 0; i < 100; i++ {
		spans = append(spans, &model.Span{
			TraceID: traceID,
			SpanID:  model.SpanID(i % distinctSpanIDs),
		})
	}
	trace := &model.Trace{Spans: spans}
	deduper := DedupeBySpanID()
	trace, err := deduper.Adjust(trace)
	require.NoError(t, err)

	assert.Len(t, trace.Spans, distinctSpanIDs, "should dedupe spans")

	var ids [distinctSpanIDs]int
	for i, span := range trace.Spans {
		ids[i] = int(span.SpanID)
	}
	assert.ElementsMatch(t, []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, ids, "should keep unique span IDs")
}
