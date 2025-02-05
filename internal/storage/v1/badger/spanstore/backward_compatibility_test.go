// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
)

// This test is for checking the backward compatibility after changing the index.
// Once dual lookup is completely removed, this test can be removed
func TestBackwardCompatibility(t *testing.T) {
	runWithBadger(t, func(store *badger.DB, t *testing.T) {
		startT := time.Now()
		tid := startT
		cache := NewCacheStore(1 * time.Hour)
		reader := NewTraceReader(store, cache, true, true)
		writer := NewSpanWriter(store, cache, 1*time.Hour)
		oldSpan := model.Span{
			TraceID: model.TraceID{
				Low:  0,
				High: 1,
			},
			SpanID:        model.SpanID(rand.Uint64()),
			OperationName: "operation-1",
			Process: &model.Process{
				ServiceName: "service",
			},
			StartTime: tid,
			Duration:  time.Duration(time.Duration(1) * time.Millisecond),
		}
		err := writer.writeSpan(&oldSpan, true)
		require.NoError(t, err)
		traces, err := reader.FindTraces(context.Background(), &spanstore.TraceQueryParameters{
			ServiceName:   "service",
			OperationName: "operation-1",
			StartTimeMin:  startT,
			StartTimeMax:  startT.Add(time.Duration(time.Millisecond * 10)),
		})
		require.NoError(t, err)
		assert.Len(t, traces, 1)
		assert.Len(t, traces[0].Spans, 1)
		assert.Equal(t, oldSpan.TraceID, traces[0].Spans[0].TraceID)
		assert.Equal(t, oldSpan.SpanID, traces[0].Spans[0].SpanID)
	})
}
