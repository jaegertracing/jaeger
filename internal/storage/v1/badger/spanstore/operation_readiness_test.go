// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"bytes"
	"testing"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

func TestOperationIndexReadiness(t *testing.T) {
	setOperationIndexGate(t, true)
	for name, value := range map[string][]byte{
		"legacy":  nil,
		"partial": (operationKinds{legacy: true, kinds: []string{"client"}}).encode(),
		"invalid": {0xff},
	} {
		t.Run(name, func(t *testing.T) {
			runWithBadger(t, func(db *badger.DB, t *testing.T) {
				span := createDummySpan()
				start := model.TimeAsEpochMicroseconds(span.StartTime)
				operationKey := createIndexKey(operationNameIndexKey, []byte(span.Process.ServiceName+span.OperationName), start, span.TraceID)
				serviceKey := createIndexKey(serviceNameIndexKey, []byte(span.Process.ServiceName), start, span.TraceID)
				require.NoError(t, db.Update(func(txn *badger.Txn) error {
					require.NoError(t, txn.Set(serviceKey, nil))
					return txn.Set(operationKey, value)
				}))
				cache := NewCacheStore(db, time.Hour)
				reader := NewTraceReader(db, cache, true)
				status := reader.OperationIndexReadiness()
				assert.False(t, status.Ready)
				assert.Equal(t, 1, status.LegacyEntries+status.PartialEntries+status.InvalidEntries)
				ops, err := reader.GetOperations(t.Context(), tracestore.OperationQueryParams{ServiceName: span.Process.ServiceName, SpanKind: "missing"})
				require.NoError(t, err)
				assert.Equal(t, []tracestore.Operation{{Name: span.OperationName}}, ops)
				if name != "invalid" {
					span.Tags = []model.KeyValue{model.String(model.SpanKindKey, "server")}
					require.NoError(t, NewSpanWriter(db, cache, time.Hour).WriteSpan(t.Context(), &span))
					restarted := NewTraceReader(db, NewCacheStore(db, time.Hour), true)
					assert.False(t, restarted.OperationIndexReadiness().Ready, "rewriting legacy data cannot recover historical kinds")
					assert.Equal(t, 1, restarted.OperationIndexReadiness().PartialEntries)
				}
				require.NoError(t, db.Update(func(txn *badger.Txn) error { return txn.Delete(operationKey) }))
				assert.False(t, NewTraceReader(db, cache, true).OperationIndexReadiness().Ready, "readiness must stay fixed until restart")
				assert.True(t, NewTraceReader(db, NewCacheStore(db, time.Hour), true).OperationIndexReadiness().Ready)
			})
		})
	}
}

func TestOperationIndexExpiredLegacyAndBackupRestore(t *testing.T) {
	setOperationIndexGate(t, true)
	runWithBadger(t, func(db *badger.DB, t *testing.T) {
		span := createDummySpan()
		start := model.TimeAsEpochMicroseconds(span.StartTime)
		key := createIndexKey(operationNameIndexKey, []byte("old-serviceold-operation"), start, span.TraceID)
		require.NoError(t, db.Update(func(txn *badger.Txn) error {
			return txn.SetEntry(&badger.Entry{Key: key, ExpiresAt: uint64(time.Now().Add(-time.Hour).Unix())})
		}))
		reader := NewTraceReader(db, NewCacheStore(db, time.Hour), true)
		assert.True(t, reader.OperationIndexReadiness().Ready, "expired legacy entries must not block readiness")
		key = createIndexKey(operationNameIndexKey, []byte(span.Process.ServiceName+span.OperationName), start, span.TraceID)
		require.NoError(t, db.Update(func(txn *badger.Txn) error {
			serviceKey := createIndexKey(serviceNameIndexKey, []byte(span.Process.ServiceName), start, span.TraceID)
			require.NoError(t, txn.Set(serviceKey, nil))
			return txn.Set(key, nil)
		}))
		var backup bytes.Buffer
		_, err := db.Backup(&backup, 0)
		require.NoError(t, err)
		restored, err := badger.Open(badger.DefaultOptions(t.TempDir()).WithLogger(nil))
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, restored.Close()) })
		require.NoError(t, restored.Load(&backup, 1))
		status := NewTraceReader(restored, NewCacheStore(restored, time.Hour), true).OperationIndexReadiness()
		assert.False(t, status.Ready)
		assert.Equal(t, 1, status.LegacyEntries)
	})
}

func TestOperationIndexReadinessIncludesOrphanEntries(t *testing.T) {
	runWithBadger(t, func(db *badger.DB, t *testing.T) {
		key := createIndexKey(operationNameIndexKey, []byte("orphan"), 1, model.TraceID{Low: 1})
		require.NoError(t, db.Update(func(txn *badger.Txn) error { return txn.Set(key, nil) }))
		status := NewTraceReader(db, NewCacheStore(db, time.Hour), true).OperationIndexReadiness()
		assert.False(t, status.Ready)
		assert.Equal(t, 1, status.LegacyEntries)
	})
}

func TestOperationIndexMalformedKeys(t *testing.T) {
	for _, prefix := range []byte{serviceNameIndexKey, operationNameIndexKey} {
		t.Run(string([]byte{prefix}), func(t *testing.T) {
			runWithBadger(t, func(db *badger.DB, t *testing.T) {
				require.NoError(t, db.Update(func(txn *badger.Txn) error {
					serviceKey := createIndexKey(serviceNameIndexKey, []byte("svc"), 1, model.TraceID{Low: 1})
					require.NoError(t, txn.Set(serviceKey, nil))
					return txn.Set(append([]byte{prefix}, []byte("svc")...), nil)
				}))
				status := NewTraceReader(db, NewCacheStore(db, time.Hour), true).OperationIndexReadiness()
				assert.False(t, status.Ready)
				require.Error(t, status.Err)
			})
		})
	}
}

func TestOperationIndexReadError(t *testing.T) {
	db, err := badger.Open(badger.DefaultOptions(t.TempDir()).WithLogger(nil))
	require.NoError(t, err)
	require.NoError(t, db.Close())
	cache := NewCacheStore(db, time.Hour)
	status := NewTraceReader(db, cache, true).OperationIndexReadiness()
	assert.False(t, status.Ready)
	require.ErrorIs(t, status.Err, badger.ErrDBClosed)
	span := createDummySpan()
	require.ErrorIs(t, NewSpanWriter(db, cache, time.Hour).WriteSpan(t.Context(), &span), badger.ErrDBClosed)
	assert.Empty(t, cache.operations)
}

func TestOperationIndexReadinessIsPerDatabase(t *testing.T) {
	setOperationIndexGate(t, true)
	var readers []*TraceReader
	for _, legacy := range []bool{false, true} {
		db, err := badger.Open(badger.DefaultOptions(t.TempDir()).WithLogger(nil))
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, db.Close()) })
		span := createDummySpan()
		span.Tags = []model.KeyValue{model.String(model.SpanKindKey, "server")}
		cache := NewCacheStore(db, time.Hour)
		require.NoError(t, NewSpanWriter(db, cache, time.Hour).WriteSpan(t.Context(), &span))
		if legacy {
			key := createIndexKey(operationNameIndexKey, []byte(span.Process.ServiceName+span.OperationName), model.TimeAsEpochMicroseconds(span.StartTime), span.TraceID)
			require.NoError(t, db.Update(func(txn *badger.Txn) error { return txn.Set(key, nil) }))
		}
		readers = append(readers, NewTraceReader(db, NewCacheStore(db, time.Hour), true))
	}
	assert.True(t, readers[0].OperationIndexReadiness().Ready)
	assert.False(t, readers[1].OperationIndexReadiness().Ready)
	for i, reader := range readers {
		span := createDummySpan()
		ops, err := reader.GetOperations(t.Context(), tracestore.OperationQueryParams{ServiceName: span.Process.ServiceName, SpanKind: "missing"})
		require.NoError(t, err)
		if i == 0 {
			assert.Empty(t, ops)
		} else {
			assert.Equal(t, []tracestore.Operation{{Name: span.OperationName}}, ops)
		}
	}
}
