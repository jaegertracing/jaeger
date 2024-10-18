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

var (
	clientSpanID  = model.NewSpanID(1)
	anotherSpanID = model.NewSpanID(11)
	keySpanKind   = "span.kind"
)

func newZipkinTrace() *model.Trace {
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

func TestZipkinSpanIDUniquifierTriggered(t *testing.T) {
	zipkinTrace := newZipkinTrace()
	deduper := ZipkinSpanIDUniquifier()
	zipkinTrace, err := deduper.Adjust(zipkinTrace)
	require.NoError(t, err)

	clientSpan := zipkinTrace.Spans[0]
	assert.Equal(t, clientSpanID, clientSpan.SpanID, "client span ID should not change")

	serverSpan := zipkinTrace.Spans[1]
	assert.Equal(t, clientSpanID+1, serverSpan.SpanID, "server span ID should be reassigned")
	assert.Equal(t, clientSpan.SpanID, serverSpan.ParentSpanID(), "client span should be server span's parent")

	thirdSpan := zipkinTrace.Spans[2]
	assert.Equal(t, anotherSpanID, thirdSpan.SpanID, "3rd span ID should not change")
	assert.Equal(t, serverSpan.SpanID, thirdSpan.ParentSpanID(), "server span should be 3rd span's parent")
}

func TestZipkinSpanIDUniquifierNotTriggered(t *testing.T) {
	zipkinTrace := newZipkinTrace()
	zipkinTrace.Spans = zipkinTrace.Spans[1:] // remove client span

	deduper := ZipkinSpanIDUniquifier()
	zipkinTrace, err := deduper.Adjust(zipkinTrace)
	require.NoError(t, err)

	serverSpanID := clientSpanID // for better readability
	serverSpan := zipkinTrace.Spans[0]
	assert.Equal(t, serverSpanID, serverSpan.SpanID, "server span ID should be unchanged")

	thirdSpan := zipkinTrace.Spans[1]
	assert.Equal(t, anotherSpanID, thirdSpan.SpanID, "3rd span ID should not change")
	assert.Equal(t, serverSpan.SpanID, thirdSpan.ParentSpanID(), "server span should be 3rd span's parent")
}

func TestZipkinSpanIDUniquifierError(t *testing.T) {
	zipkinTrace := newZipkinTrace()

	maxID := int64(-1)
	assert.Equal(t, maxSpanID, model.NewSpanID(uint64(maxID)), "maxSpanID must be 2^64-1")

	deduper := &spanIDDeduper{trace: zipkinTrace}
	deduper.groupSpansByID()
	deduper.maxUsedID = maxSpanID - 1
	deduper.uniquifyServerSpanIDs()
	if assert.Len(t, zipkinTrace.Spans[1].Warnings, 1) {
		assert.Equal(t, "cannot assign unique span ID, too many spans in the trace", zipkinTrace.Spans[1].Warnings[0])
	}
}
