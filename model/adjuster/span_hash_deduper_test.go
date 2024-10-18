// Copyright (c) 2024 The Jaeger Authors.
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
				SpanID:  anotherSpanID,
				Tags: model.KeyValues{
					// span.kind = server
					model.String(keySpanKind, trace.SpanKindServer.String()),
				},
			},
			{
				TraceID:    traceID,
				SpanID:     anotherSpanID, // same ID as before, but different metadata
				References: []model.SpanRef{model.NewChildOfRef(traceID, clientSpanID)},
			},
		},
	}
}

func getSpanIDs(spans []*model.Span) []int {
	ids := make([]int, len(spans))
	for i, span := range spans {
		ids[i] = int(span.SpanID)
	}
	return ids
}

func TestDedupeBySpanHashTriggers(t *testing.T) {
	spansTrace := newDuplicatedSpansTrace()
	deduper := DedupeBySpanHash()
	spansTrace, err := deduper.Adjust(spansTrace)
	require.NoError(t, err)

	assert.Len(t, spansTrace.Spans, 2, "should dedupe spans")

	ids := getSpanIDs(spansTrace.Spans)
	assert.ElementsMatch(t, []int{int(clientSpanID), int(anotherSpanID)}, ids, "should keep unique span IDs")
}

func TestDedupeBySpanHashNotTriggered(t *testing.T) {
	spansTrace := newUniqueSpansTrace()
	deduper := DedupeBySpanHash()
	spansTrace, err := deduper.Adjust(spansTrace)
	require.NoError(t, err)

	assert.Len(t, spansTrace.Spans, 2, "should not dedupe spans")

	ids := getSpanIDs(spansTrace.Spans)
	assert.ElementsMatch(t, []int{int(anotherSpanID), int(anotherSpanID)}, ids, "should keep unique span IDs")
	assert.NotEqual(t, spansTrace.Spans[0], spansTrace.Spans[1], "should keep unique hashcodes")
}

func TestDedupeBySpanHashEmpty(t *testing.T) {
	traceInstance := &model.Trace{}
	deduper := DedupeBySpanHash()
	traceInstance, err := deduper.Adjust(traceInstance)
	require.NoError(t, err)

	assert.Empty(t, traceInstance.Spans, "should be empty")
}

func TestDedupeBySpanHashManyManySpans(t *testing.T) {
	traceID := model.NewTraceID(0, 42)
	spans := make([]*model.Span, 0, 100)
	const distinctSpanIDs = 10
	for i := 0; i < 100; i++ {
		spans = append(spans, &model.Span{
			TraceID: traceID,
			SpanID:  model.SpanID(i % distinctSpanIDs),
		})
	}
	traceInstance := &model.Trace{Spans: spans}
	deduper := DedupeBySpanHash()
	traceInstance, err := deduper.Adjust(traceInstance)
	require.NoError(t, err)

	assert.Len(t, traceInstance.Spans, distinctSpanIDs, "should dedupe spans")

	ids := getSpanIDs(traceInstance.Spans)
	assert.ElementsMatch(t, []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, ids, "should keep unique span IDs")
}
