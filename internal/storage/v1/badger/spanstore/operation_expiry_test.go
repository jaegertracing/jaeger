// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"testing"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

func TestOperationExpiryAfterRestart(t *testing.T) {
	setOperationIndexGate(t, true)
	for _, shorterLast := range []bool{true, false} {
		t.Run(map[bool]string{true: "shorter last", false: "longer last"}[shorterLast], func(t *testing.T) {
			opts := badger.DefaultOptions(t.TempDir()).WithLogger(nil)
			db, err := badger.Open(opts)
			require.NoError(t, err)
			cache := NewCacheStore(db, time.Hour)
			span := createDummySpan()
			first, second := time.Hour, time.Second
			if !shorterLast {
				first, second = second, first
			}
			span.Tags = []model.KeyValue{model.String(model.SpanKindKey, "client")}
			require.NoError(t, NewSpanWriter(db, cache, first).WriteSpan(t.Context(), &span))
			span.SpanID++
			span.Tags = []model.KeyValue{model.String(model.SpanKindKey, "server")}
			require.NoError(t, NewSpanWriter(db, cache, second).WriteSpan(t.Context(), &span))
			// Another operation sharing the service index must not shorten its expiry.
			span.OperationName = "short-lived"
			span.SpanID++
			require.NoError(t, NewSpanWriter(db, cache, time.Second).WriteSpan(t.Context(), &span))
			require.NoError(t, db.Close())
			deadline := time.Now().Unix() + 2
			require.Eventually(t, func() bool { return time.Now().Unix() >= deadline }, 4*time.Second, 10*time.Millisecond)
			db, err = badger.Open(opts)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, db.Close()) })
			reader := NewTraceReader(db, NewCacheStore(db, time.Hour), true)
			assert.True(t, reader.OperationIndexReadiness().Ready)
			services, err := reader.GetServices(t.Context())
			require.NoError(t, err)
			assert.Equal(t, []string{span.Process.ServiceName}, services)
			ops, err := reader.GetOperations(t.Context(), tracestore.OperationQueryParams{ServiceName: span.Process.ServiceName})
			require.NoError(t, err)
			assert.Equal(t, []tracestore.Operation{
				{Name: "operation", SpanKind: "client"},
				{Name: "operation", SpanKind: "server"},
			}, ops)
		})
	}
}

func TestOperationIndexNoExpiry(t *testing.T) {
	setOperationIndexGate(t, true)
	runWithBadger(t, func(db *badger.DB, t *testing.T) {
		span := createDummySpan()
		start := model.TimeAsEpochMicroseconds(span.StartTime)
		key := createIndexKey(operationNameIndexKey, []byte(span.Process.ServiceName+span.OperationName), start, span.TraceID)
		require.NoError(t, db.Update(func(txn *badger.Txn) error {
			return txn.Set(key, (operationKinds{kinds: []string{"client"}}).encode())
		}))
		cache := NewCacheStore(db, time.Hour)
		span.Tags = []model.KeyValue{model.String(model.SpanKindKey, "server")}
		require.NoError(t, NewSpanWriter(db, cache, time.Hour).WriteSpan(t.Context(), &span))
		require.NoError(t, db.View(func(txn *badger.Txn) error {
			for _, index := range [][]byte{key, createIndexKey(serviceNameIndexKey, []byte(span.Process.ServiceName), start, span.TraceID)} {
				item, err := txn.Get(index)
				require.NoError(t, err)
				assert.Zero(t, item.ExpiresAt())
			}
			return nil
		}))
		reader := NewTraceReader(db, NewCacheStore(db, time.Hour), true)
		ops, err := reader.GetOperations(t.Context(), tracestore.OperationQueryParams{ServiceName: span.Process.ServiceName})
		require.NoError(t, err)
		assert.Equal(t, []tracestore.Operation{{Name: span.OperationName, SpanKind: "client"}, {Name: span.OperationName, SpanKind: "server"}}, ops)
	})
}

func TestOperationCacheRetainsLaterCommits(t *testing.T) {
	cache := NewCacheStore(nil, time.Hour)
	for _, expiry := range []uint64{100, 200, 150, 0, 300} {
		cache.updateOperations("service", "operation", []string{"server", "client"}, expiry, expiry)
	}
	assert.Zero(t, cache.services["service"])
	for _, expiry := range cache.operations["service"] {
		assert.Zero(t, expiry)
	}
	cache.updateOperations("service", "another", []string{"server"}, 100, 100)
	ops, err := cache.queryOperations(tracestore.OperationQueryParams{ServiceName: "service"}, true)
	require.NoError(t, err)
	assert.Equal(t, []tracestore.Operation{{Name: "operation", SpanKind: "client"}, {Name: "operation", SpanKind: "server"}}, ops)
	cache.AddService("service", 100)
	assert.Zero(t, cache.services["service"])
}
