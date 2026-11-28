// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/featuregate"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

func TestWriterPersistsOperationKind(t *testing.T) {
	runWithBadger(t, func(db *badger.DB, t *testing.T) {
		span := createDummySpan()
		span.Tags = []model.KeyValue{model.String("span.kind", "server")}
		writer := NewSpanWriter(db, NewCacheStore(db, time.Hour), time.Hour)
		require.NoError(t, writer.WriteSpan(context.Background(), &span))
		key := createIndexKey(operationNameIndexKey, []byte(span.Process.ServiceName+span.OperationName), model.TimeAsEpochMicroseconds(span.StartTime), span.TraceID)
		require.NoError(t, db.View(func(txn *badger.Txn) error {
			item, err := txn.Get(key)
			require.NoError(t, err)
			value, err := item.ValueCopy(nil)
			require.NoError(t, err)
			require.NotEmpty(t, value, "operation index must retain kind for cache rebuilds")
			kinds, err := decodeOperationKinds(value)
			require.NoError(t, err)
			assert.Equal(t, operationKinds{kinds: []string{"server"}}, kinds)
			return nil
		}))
	})
}

func setOperationIndexGate(t *testing.T, enabled bool) {
	t.Helper()
	previous := operationIndexGate.IsEnabled()
	require.NoError(t, featuregate.GlobalRegistry().Set(operationIndexGate.ID(), enabled))
	t.Cleanup(func() {
		require.NoError(t, featuregate.GlobalRegistry().Set(operationIndexGate.ID(), previous))
	})
}

func TestOperationKindsEncoding(t *testing.T) {
	for _, kinds := range []operationKinds{
		{kinds: []string{""}},
		{kinds: []string{"", "client", "server", "unknown\x00\xff"}},
		{legacy: true, kinds: []string{"server"}},
	} {
		decoded, err := decodeOperationKinds(kinds.encode())
		require.NoError(t, err)
		assert.Equal(t, kinds, decoded)
	}
	legacy, err := decodeOperationKinds(nil)
	require.NoError(t, err)
	assert.Equal(t, operationKinds{legacy: true}, legacy)
	for name, value := range map[string][]byte{
		"short header": {1},
		"version":      {2, 0, 0, 0, 0, 0},
		"flags":        {1, 2, 0, 0, 0, 0},
		"no kinds":     {1, 0},
		"short length": {1, 0, 0},
		"short kind":   {1, 0, 0xff, 0xff, 0xff, 0xff},
		"duplicate":    (operationKinds{kinds: []string{"server", "server"}}).encode(),
		"unsorted":     (operationKinds{kinds: []string{"server", "client"}}).encode(),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := decodeOperationKinds(value)
			require.Error(t, err)
		})
	}
}

func TestOperationsWithKindsAndPrefill(t *testing.T) {
	setOperationIndexGate(t, true)
	runWithBadger(t, func(db *badger.DB, t *testing.T) {
		cache := NewCacheStore(db, time.Hour)
		reader := NewTraceReader(db, cache, true)
		writer := NewSpanWriter(db, cache, time.Hour)
		span := createDummySpan()
		service := span.Process.ServiceName
		span.Process.Tags = []model.KeyValue{model.String(model.SpanKindKey, "process-kind")}
		var want []tracestore.Operation
		for i, tags := range [][]model.KeyValue{
			nil,
			{model.String(model.SpanKindKey, "")},
			{model.String(model.SpanKindKey, "client")},
			{model.String(model.SpanKindKey, "server"), model.String(model.SpanKindKey, "ignored")},
			{model.String(model.SpanKindKey, "custom\x00\xff")},
			{model.Int64(model.SpanKindKey, 42)},
		} {
			span.SpanID = model.SpanID(i)
			span.Tags = tags
			require.NoError(t, writer.WriteSpan(t.Context(), &span))
			if i != 1 {
				want = append(want, tracestore.Operation{Name: span.OperationName, SpanKind: spanKind(&span)})
			}
		}
		slices.SortFunc(want, func(a, b tracestore.Operation) int { return cmp.Compare(a.SpanKind, b.SpanKind) })
		for _, current := range []*TraceReader{reader, NewTraceReader(db, NewCacheStore(db, time.Hour), true)} {
			assert.True(t, current.OperationIndexReadiness().Ready)
			ops, err := current.GetOperations(t.Context(), tracestore.OperationQueryParams{ServiceName: service})
			require.NoError(t, err)
			assert.Equal(t, want, ops)
			for _, kind := range []string{"client", "server", "missing"} {
				ops, err = current.GetOperations(t.Context(), tracestore.OperationQueryParams{ServiceName: service, SpanKind: kind})
				require.NoError(t, err)
				if kind == "missing" {
					assert.Empty(t, ops)
				} else {
					assert.Equal(t, []tracestore.Operation{{Name: span.OperationName, SpanKind: kind}}, ops)
				}
			}
			ops, err = current.GetOperations(t.Context(), tracestore.OperationQueryParams{ServiceName: "missing"})
			require.NoError(t, err)
			assert.Empty(t, ops)
			ids, err := current.FindTraceIDs(t.Context(), &spanstore.TraceQueryParameters{
				ServiceName: service, OperationName: span.OperationName,
				StartTimeMin: span.StartTime.Add(-time.Second), StartTimeMax: span.StartTime.Add(time.Second),
			})
			require.NoError(t, err)
			assert.Equal(t, []model.TraceID{span.TraceID}, ids)
		}
		setOperationIndexGate(t, false)
		ops, err := reader.GetOperations(t.Context(), tracestore.OperationQueryParams{ServiceName: service, SpanKind: "missing"})
		require.NoError(t, err)
		assert.Equal(t, []tracestore.Operation{{Name: span.OperationName}}, ops)
	})
}

func TestOperationIndexConcurrentWrites(t *testing.T) {
	setOperationIndexGate(t, true)
	runWithBadger(t, func(db *badger.DB, t *testing.T) {
		cache := NewCacheStore(db, time.Hour)
		reader := NewTraceReader(db, cache, true)
		writer := NewSpanWriter(db, cache, time.Hour)
		span := createDummySpan()
		const writers = 16
		var group sync.WaitGroup
		errors := make(chan error, writers)
		start := make(chan struct{})
		for i := range writers {
			group.Go(func() {
				local := span
				local.SpanID = model.SpanID(i)
				local.Tags = []model.KeyValue{model.String(model.SpanKindKey, fmt.Sprintf("kind-%02d", i))}
				<-start
				errors <- writer.WriteSpan(t.Context(), &local)
			})
		}
		close(start)
		group.Wait()
		close(errors)
		for err := range errors {
			require.NoError(t, err)
		}
		for _, current := range []*TraceReader{reader, NewTraceReader(db, NewCacheStore(db, time.Hour), true)} {
			ops, err := current.GetOperations(t.Context(), tracestore.OperationQueryParams{ServiceName: span.Process.ServiceName})
			require.NoError(t, err)
			require.Len(t, ops, writers)
			for i, op := range ops {
				assert.Equal(t, fmt.Sprintf("kind-%02d", i), op.SpanKind)
			}
		}
	})
}

func TestOperationIndexFailedWriteDoesNotUpdateCache(t *testing.T) {
	runWithBadger(t, func(db *badger.DB, t *testing.T) {
		cache := NewCacheStore(db, time.Hour)
		writer := NewSpanWriter(db, cache, time.Hour)
		span := createDummySpan()
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		require.ErrorIs(t, writer.WriteSpan(ctx, &span), context.Canceled)
		assert.Empty(t, cache.services)
		assert.Empty(t, cache.operations)
		key := createIndexKey(operationNameIndexKey, []byte(span.Process.ServiceName+span.OperationName), model.TimeAsEpochMicroseconds(span.StartTime), span.TraceID)
		require.NoError(t, db.Update(func(txn *badger.Txn) error { return txn.Set(key, []byte{0xff}) }))
		require.ErrorContains(t, writer.WriteSpan(t.Context(), &span), "decode operation index")
		assert.Empty(t, cache.operations)
	})
}

func TestOperationIndexTransactionErrors(t *testing.T) {
	runWithBadger(t, func(db *badger.DB, t *testing.T) {
		txn := db.NewTransaction(true)
		txn.Discard()
		_, _, err := mergeOperationIndex(txn, []byte("operation"), "server", 0)
		require.ErrorIs(t, err, badger.ErrDiscardedTxn)
		_, err = mergeServiceExpiry(txn, []byte("service"), 0)
		require.ErrorIs(t, err, badger.ErrDiscardedTxn)
		span := createDummySpan()
		span.OperationName = strings.Repeat("x", 1<<16)
		cache := NewCacheStore(db, time.Hour)
		require.ErrorContains(t, NewSpanWriter(db, cache, time.Hour).WriteSpan(t.Context(), &span), "Key with size")
		assert.Empty(t, cache.services)
		assert.Empty(t, cache.operations)
	})
	t.Run("span exceeds transaction limit", func(t *testing.T) {
		db, err := badger.Open(badger.DefaultOptions(t.TempDir()).WithMemTableSize(1 << 20).WithValueThreshold(1024).WithLogger(nil))
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, db.Close()) })
		cache := NewCacheStore(db, time.Hour)
		span := createDummySpan()
		span.Tags = make([]model.KeyValue, 3000)
		for i := range span.Tags {
			span.Tags[i] = model.String(fmt.Sprintf("key-%d", i), strings.Repeat("x", 64))
		}
		require.ErrorIs(t, NewSpanWriter(db, cache, time.Hour).WriteSpan(t.Context(), &span), badger.ErrTxnTooBig)
		assert.Empty(t, cache.services)
		assert.Empty(t, cache.operations)
	})
}
