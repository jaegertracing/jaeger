// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/model"
)

/*
	Additional cache store tests that need to access internal parts. As such, package must be spanstore and not spanstore_test
*/

var spanKinds = []model.SpanKind{
	model.SpanKindUnspecified,
	model.SpanKindInternal,
	model.SpanKindClient,
	model.SpanKindServer,
	model.SpanKindProducer,
	model.SpanKindConsumer,
}

func TestExpiredItems(t *testing.T) {
	runWithBadger(t, func(store *badger.DB, t *testing.T) {
		cache := NewCacheStore(store, time.Duration(-1*time.Hour), false)

		expireTime := uint64(time.Now().Add(cache.ttl).Unix())

		// Expired service

		cache.Update("service1", "op1", model.SpanKindUnspecified, expireTime)
		cache.Update("service1", "op2", model.SpanKindUnspecified, expireTime)

		services, err := cache.GetServices()
		require.NoError(t, err)
		assert.Empty(t, services) // Everything should be expired

		// Expired service for operations

		cache.Update("service1", "op1", model.SpanKindUnspecified, expireTime)
		cache.Update("service1", "op2", model.SpanKindUnspecified, expireTime)

		operations, err := cache.GetOperations("service1", "")
		require.NoError(t, err)
		assert.Empty(t, operations) // Everything should be expired

		// Expired operations, stable service

		cache.Update("service1", "op1", model.SpanKindUnspecified, expireTime)
		cache.Update("service1", "op2", model.SpanKindUnspecified, expireTime)

		cache.services["service1"] = uint64(time.Now().Unix() + 1e10)

		operations, err = cache.GetOperations("service1", "")
		require.NoError(t, err)
		assert.Empty(t, operations) // Everything should be expired
	})
}

func TestOldReads(t *testing.T) {
	runWithBadger(t, func(store *badger.DB, t *testing.T) {
		timeNow := model.TimeAsEpochMicroseconds(time.Now())
		s1Key := createIndexKey(serviceNameIndexKey, []byte("service1"), timeNow, model.TraceID{High: 0, Low: 0})
		s1o1Key := createIndexKey(operationNameIndexKey, []byte("service1operation1"), timeNow, model.TraceID{High: 0, Low: 0})

		tid := time.Now().Add(1 * time.Minute)

		writer := func() {
			store.Update(func(txn *badger.Txn) error {
				txn.SetEntry(&badger.Entry{
					Key:       s1Key,
					ExpiresAt: uint64(tid.Unix()),
				})
				txn.SetEntry(&badger.Entry{
					Key:       s1o1Key,
					ExpiresAt: uint64(tid.Unix()),
				})
				return nil
			})
		}

		cache := NewCacheStore(store, time.Duration(-1*time.Hour), false)
		writer()

		nuTid := tid.Add(1 * time.Hour)

		cache.Update("service1", "operation1", model.SpanKindUnspecified, uint64(tid.Unix()))
		cache.services["service1"] = uint64(nuTid.Unix())
		cache.operations["service1"][model.SpanKindUnspecified]["operation1"] = uint64(nuTid.Unix())

		cache.populateCaches()

		// Now make sure we didn't use the older timestamps from the DB
		assert.Equal(t, uint64(nuTid.Unix()), cache.services["service1"])
		assert.Equal(t, uint64(nuTid.Unix()), cache.operations["service1"][model.SpanKindUnspecified]["operation1"])
	})
}

func TestCacheStore_GetOperations(t *testing.T) {
	runWithBadger(t, func(store *badger.DB, t *testing.T) {
		cache := NewCacheStore(store, 1*time.Hour, false)
		expireTime := uint64(time.Now().Add(cache.ttl).Unix())
		cache.services = make(map[string]uint64)
		serviceName := "service1"
		operationName := "op1"
		cache.services[serviceName] = uint64(time.Now().Unix() + 1e10)
		cache.operations = make(map[string]map[model.SpanKind]map[string]uint64)
		cache.operations[serviceName] = make(map[model.SpanKind]map[string]uint64)
		for i := 0; i <= 5; i++ {
			cache.operations[serviceName][spanKinds[i]] = make(map[string]uint64)
			cache.operations[serviceName][spanKinds[i]][operationName] = expireTime
		}
		operations, err := cache.GetOperations(serviceName, "")
		require.NoError(t, err)
		assert.Len(t, operations, 6)
		var kinds []string
		for i := 0; i <= 5; i++ {
			kinds = append(kinds, string(spanKinds[i]))
		}
		// This is necessary as we want to check whether the result is sorted or not
		sort.Strings(kinds)
		for i := 0; i <= 5; i++ {
			assert.Equal(t, kinds[i], operations[i].SpanKind)
			assert.Equal(t, operationName, operations[i].Name)
			if i != 0 {
				k := kinds[i]
				singleKindOperations, err := cache.GetOperations(serviceName, k)
				require.NoError(t, err)
				assert.Len(t, singleKindOperations, 1)
				assert.Equal(t, kinds[i], singleKindOperations[0].SpanKind)
				assert.Equal(t, operationName, singleKindOperations[0].Name)
			}
		}
	})
}

func TestCacheStore_Update(t *testing.T) {
	runWithBadger(t, func(store *badger.DB, t *testing.T) {
		cache := NewCacheStore(store, 1*time.Hour, false)
		expireTime := uint64(time.Now().Add(cache.ttl).Unix())
		serviceName := "service1"
		operationName := "op1"
		for i := 0; i <= 5; i++ {
			cache.Update(serviceName, operationName, spanKinds[i], expireTime)
			assert.Equal(t, expireTime, cache.operations[serviceName][spanKinds[i]][operationName])
		}
	})
}

func TestCacheStore_Prefill(t *testing.T) {
	runWithBadger(t, func(store *badger.DB, t *testing.T) {
		cache := NewCacheStore(store, 1*time.Hour, true)
		writer := NewSpanWriter(store, cache, 1*time.Hour)
		var spans []*model.Span
		// Write a span without kind also
		spanWithoutKind := createDummySpanWithKind("service0", "op0", model.SpanKindUnspecified, false)
		spans = append(spans, spanWithoutKind)
		err := writer.WriteSpan(context.Background(), spanWithoutKind)
		require.NoError(t, err)
		for i := 1; i < 6; i++ {
			service := fmt.Sprintf("service%d", i)
			operation := fmt.Sprintf("op%d", i)
			span := createDummySpanWithKind(service, operation, spanKinds[i], true)
			spans = append(spans, span)
			err = writer.WriteSpan(context.Background(), span)
			require.NoError(t, err)
		}
		// Create a new cache for testing prefill as old span will consist the data from update called from WriteSpan
		newCache := NewCacheStore(store, 1*time.Hour, true)
		for i, span := range spans {
			_, foundService := newCache.services[span.Process.ServiceName]
			assert.True(t, foundService)
			_, foundOperation := newCache.operations[span.Process.ServiceName][spanKinds[i]][span.OperationName]
			assert.True(t, foundOperation)
		}
	})
}

// Test Case for old data
func TestCacheStore_WhenValueIsNil(t *testing.T) {
	runWithBadger(t, func(store *badger.DB, t *testing.T) {
		cache := NewCacheStore(store, 1*time.Hour, true)
		w := NewSpanWriter(store, cache, 1*time.Hour)
		var entriesToStore []*badger.Entry
		timeNow := model.TimeAsEpochMicroseconds(time.Now())
		expireTime := uint64(time.Now().Add(cache.ttl).Unix())
		entriesToStore = append(entriesToStore, w.createBadgerEntry(createIndexKey(serviceNameIndexKey, []byte("service"), timeNow, model.TraceID{High: 0, Low: 0}), nil, expireTime))
		entriesToStore = append(entriesToStore, w.createBadgerEntry(createIndexKey(operationNameIndexKey, []byte("serviceoperation"), timeNow, model.TraceID{High: 0, Low: 0}), nil, expireTime))
		err := store.Update(func(txn *badger.Txn) error {
			err := txn.SetEntry(entriesToStore[0])
			require.NoError(t, err)
			err = txn.SetEntry(entriesToStore[1])
			require.NoError(t, err)
			return nil
		})
		require.NoError(t, err)
		newCache := NewCacheStore(store, 1*time.Hour, true)
		_, foundService := newCache.services["service"]
		assert.True(t, foundService)
		_, foundOperation := newCache.operations["service"][model.SpanKindUnspecified]["operation"]
		assert.True(t, foundOperation)
	})
}

func TestCacheStore_GetOperationsReturnsEmptyOperationsWithWrongSpanKind(t *testing.T) {
	runWithBadger(t, func(store *badger.DB, t *testing.T) {
		cache := NewCacheStore(store, 1*time.Hour, true)
		cache.services = make(map[string]uint64)
		cache.services["service1"] = uint64(time.Now().Unix())
		operations, err := cache.GetOperations("service1", "a")
		require.NoError(t, err)
		assert.Empty(t, operations)
	})
}

// Code Coverage Test
func TestCacheStore_WrongSpanKindFromBadger(t *testing.T) {
	runWithBadger(t, func(store *badger.DB, t *testing.T) {
		cache := NewCacheStore(store, 1*time.Hour, true)
		writer := NewSpanWriter(store, cache, 1*time.Hour)
		span := createDummySpanWithKind("service", "operation", model.SpanKind("New Kind"), true)
		err := writer.WriteSpan(context.Background(), span)
		require.NoError(t, err)
		newCache := NewCacheStore(store, 1*time.Hour, true)
		_, foundService := newCache.services[span.Process.ServiceName]
		assert.True(t, foundService)
		_, foundOperation := newCache.operations[span.Process.ServiceName][model.SpanKindUnspecified][span.OperationName]
		assert.True(t, foundOperation)
	})
}

func TestCacheStore_GetOperationsSameKind(t *testing.T) {
	runWithBadger(t, func(store *badger.DB, t *testing.T) {
		cache := NewCacheStore(store, 1*time.Hour, true)
		writer := NewSpanWriter(store, cache, 1*time.Hour)
		for i := 0; i < 5; i++ {
			service := "service"
			operation := fmt.Sprintf("op%d", i)
			span := createDummySpanWithKind(service, operation, model.SpanKindServer, true)
			err := writer.WriteSpan(context.Background(), span)
			require.NoError(t, err)
		}
		operations, err := cache.GetOperations("service", string(model.SpanKindServer))
		require.NoError(t, err)
		assert.Len(t, operations, 5)
		for i, operation := range operations {
			assert.Equal(t, fmt.Sprintf("op%d", i), operation.Name)
			assert.Equal(t, string(model.SpanKindServer), operation.SpanKind)
		}
	})
}

// func runFactoryTest(tb testing.TB, test func(tb testing.TB, sw spanstore.Writer, sr spanstore.Reader)) {
func runWithBadger(t *testing.T, test func(store *badger.DB, t *testing.T)) {
	opts := badger.DefaultOptions("")

	opts.SyncWrites = false
	dir := t.TempDir()
	opts.Dir = dir
	opts.ValueDir = dir

	store, err := badger.Open(opts)
	defer func() {
		store.Close()
	}()

	require.NoError(t, err)

	test(store, t)
}

func createDummySpanWithKind(service string, operation string, kind model.SpanKind, includeSpanKind bool) *model.Span {
	var tags model.KeyValues
	if includeSpanKind {
		tags = model.KeyValues{
			model.KeyValue{
				Key:   "span.kind",
				VType: model.StringType,
				VStr:  string(kind),
			},
		}
	} else {
		tags = model.KeyValues{}
	}
	tid := time.Now()
	testSpan := model.Span{
		TraceID: model.TraceID{
			Low:  uint64(0),
			High: 1,
		},
		SpanID:        model.SpanID(0),
		OperationName: operation,
		Process: &model.Process{
			ServiceName: service,
		},
		StartTime: tid.Add(1 * time.Millisecond),
		Duration:  1 * time.Millisecond,
		Tags:      tags,
	}
	return &testSpan
}
