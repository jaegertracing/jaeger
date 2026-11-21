// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core/dbmodel"
)

func TestSpanWriter_ServiceDocumentsPerIndex(t *testing.T) {
	firstStart := time.Date(2026, 10, 18, 23, 59, 0, 0, time.UTC)
	for _, tc := range []struct {
		name               string
		secondStart        time.Time
		separateBatches    bool
		wantServiceIndices []string
		wantSpanIndices    []string
	}{
		{
			name:               "same index in one batch",
			secondStart:        firstStart.Add(time.Second),
			wantServiceIndices: []string{"jaeger-service-2026-10-18"},
			wantSpanIndices:    []string{"jaeger-span-2026-10-18", "jaeger-span-2026-10-18"},
		},
		{
			name:               "same index in separate batches",
			secondStart:        firstStart.Add(time.Second),
			separateBatches:    true,
			wantServiceIndices: []string{"jaeger-service-2026-10-18"},
			wantSpanIndices:    []string{"jaeger-span-2026-10-18", "jaeger-span-2026-10-18"},
		},
		{
			name:               "different indices in one batch",
			secondStart:        firstStart.Add(2 * time.Minute),
			wantServiceIndices: []string{"jaeger-service-2026-10-18", "jaeger-service-2026-10-19"},
			wantSpanIndices:    []string{"jaeger-span-2026-10-18", "jaeger-span-2026-10-19"},
		},
		{
			name:               "different indices in separate batches",
			secondStart:        firstStart.Add(2 * time.Minute),
			separateBatches:    true,
			wantServiceIndices: []string{"jaeger-service-2026-10-18", "jaeger-service-2026-10-19"},
			wantSpanIndices:    []string{"jaeger-span-2026-10-18", "jaeger-span-2026-10-19"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withSpanWriter(func(w *spanWriterTest) {
				service := dbmodel.Process{ServiceName: "shop"}
				spans := []dbmodel.Span{
					{
						TraceID:       "0000000000000001",
						SpanID:        "0000000000000001",
						OperationName: "checkout",
						Process:       service,
						StartTime:     model.TimeAsEpochMicroseconds(firstStart),
					},
					{
						TraceID:       "0000000000000002",
						SpanID:        "0000000000000002",
						OperationName: "checkout",
						Process:       service,
						StartTime:     model.TimeAsEpochMicroseconds(tc.secondStart),
					},
				}
				if tc.separateBatches {
					require.NoError(t, w.writer.WriteSpans(context.Background(), spans[:1]))
					require.NoError(t, w.writer.WriteSpans(context.Background(), spans[1:]))
				} else {
					require.NoError(t, w.writer.WriteSpans(context.Background(), spans))
				}

				var serviceIndices, spanIndices []string
				for _, item := range w.batchWriter.items {
					switch body := item.Body.(type) {
					case dbmodel.Service:
						assert.Equal(t, dbmodel.Service{ServiceName: "shop", OperationName: "checkout"}, body)
						serviceIndices = append(serviceIndices, item.Index)
					case json.RawMessage:
						spanIndices = append(spanIndices, item.Index)
					default:
						t.Fatalf("unexpected bulk item body type %T", item.Body)
					}
				}
				assert.ElementsMatch(t, tc.wantSpanIndices, spanIndices, "both spans must be written")
				assert.ElementsMatch(t, tc.wantServiceIndices, serviceIndices, "each destination index needs its own service/operation document")
			})
		})
	}
}
