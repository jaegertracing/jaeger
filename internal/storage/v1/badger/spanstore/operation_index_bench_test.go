// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger-idl/model/v1"
)

func BenchmarkOperationIndex(b *testing.B) {
	open := func(b *testing.B) *badger.DB {
		b.Helper()
		db, err := badger.Open(badger.DefaultOptions(b.TempDir()).WithLogger(nil))
		require.NoError(b, err)
		b.Cleanup(func() { require.NoError(b, db.Close()) })
		return db
	}
	for _, parallel := range []bool{false, true} {
		b.Run(fmt.Sprintf("write/parallel=%t", parallel), func(b *testing.B) {
			db := open(b)
			writer := NewSpanWriter(db, NewCacheStore(db, time.Hour), time.Hour)
			var sequence atomic.Uint64
			write := func() {
				i := sequence.Add(1)
				span := createDummySpan()
				span.StartTime = time.Unix(1_700_000_000, 0)
				span.SpanID = model.SpanID(i)
				span.Tags = []model.KeyValue{model.String(model.SpanKindKey, []string{"client", "server", "producer"}[i%3])}
				require.NoError(b, writer.WriteSpan(b.Context(), &span))
			}
			b.ReportAllocs()
			b.ResetTimer()
			if parallel {
				b.RunParallel(func(pb *testing.PB) {
					for pb.Next() {
						write()
					}
				})
			} else {
				for b.Loop() {
					write()
				}
			}
		})
	}
	b.Run("prefill/6000-spans", func(b *testing.B) {
		db := open(b)
		writer := NewSpanWriter(db, NewCacheStore(db, time.Hour), time.Hour)
		for i := range 6000 {
			span := createDummySpan()
			span.Process.ServiceName = fmt.Sprintf("checkout-service-%02d", i%20)
			span.OperationName = fmt.Sprintf("POST /api/v1/orders/operation-%03d", (i/20)%100)
			span.Tags = []model.KeyValue{model.String(model.SpanKindKey, []string{"client", "server", "producer"}[i/2000])}
			require.NoError(b, writer.WriteSpan(b.Context(), &span))
		}
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			reader := NewTraceReader(db, NewCacheStore(db, time.Hour), true)
			if !reader.OperationIndexReadiness().Ready {
				b.Fatal(reader.OperationIndexReadiness())
			}
		}
	})
	b.Run("cache/3000-pairs", func(b *testing.B) {
		names := make([]string, 1000)
		for i := range names {
			names[i] = fmt.Sprintf("POST /api/v1/orders/operation-%03d", i)
		}
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			cache := NewCacheStore(nil, time.Hour)
			for _, name := range names {
				cache.updateOperations("checkout-service", name, []string{"client", "server", "producer"}, 0, 0)
			}
		}
	})
}
