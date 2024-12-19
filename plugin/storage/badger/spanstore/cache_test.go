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
	"go.opentelemetry.io/otel/trace"

	"github.com/jaegertracing/jaeger/model"
)

/*
	Additional cache store tests that need to access internal parts. As such, package must be spanstore and not spanstore_test
*/

func TestExpiredItems(t *testing.T) {
	runWithBadger(t, func(store *badger.DB, t *testing.T) {
		cache := NewCacheStore(store, time.Duration(-1*time.Hour), false)

		expireTime := uint64(time.Now().Add(cache.ttl).Unix())

		// Expired service

		cache.Update("service1", "op1", trace.SpanKindUnspecified, expireTime)
		cache.Update("service1", "op2", trace.SpanKindUnspecified, expireTime)

		services, err := cache.GetServices()
		require.NoError(t, err)
		assert.Empty(t, services) // Everything should be expired

		// Expired service for operations

		cache.Update("service1", "op1", trace.SpanKindUnspecified, expireTime)
		cache.Update("service1", "op2", trace.SpanKindUnspecified, expireTime)

		operations, err := cache.GetOperations("service1", nil)
		require.NoError(t, err)
		assert.Empty(t, operations) // Everything should be expired

		// Expired operations, stable service

		cache.Update("service1", "op1", trace.SpanKindUnspecified, expireTime)
		cache.Update("service1", "op2", trace.SpanKindUnspecified, expireTime)

		cache.services["service1"] = uint64(time.Now().Unix() + 1e10)

		operations, err = cache.GetOperations("service1", nil)
		require.NoError(t, err)
		assert.Empty(t, operations) // Everything should be expired
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
		cache.operations = make(map[string]map[trace.SpanKind]map[string]uint64)
		cache.operations[serviceName] = make(map[trace.SpanKind]map[string]uint64)
		for i := 0; i <= 5; i++ {
			cache.operations[serviceName][trace.SpanKind(i)] = make(map[string]uint64)
			cache.operations[serviceName][trace.SpanKind(i)][operationName] = expireTime
		}
		operations, err := cache.GetOperations(serviceName, nil)
		require.NoError(t, err)
		assert.Len(t, operations, 6)
		var kinds []string
		for i := 0; i <= 5; i++ {
			kinds = append(kinds, trace.SpanKind(i).String())
		}
		// This is necessary as we want to check whether the result is sorted or not
		sort.Strings(kinds)
		for i := 0; i <= 5; i++ {
			assert.Equal(t, kinds[i], operations[i].SpanKind)
			assert.Equal(t, operationName, operations[i].Name)
			k := trace.SpanKind(i)
			kp := &k
			singleKindOperations, err := cache.GetOperations(serviceName, kp)
			require.NoError(t, err)
			assert.Len(t, singleKindOperations, 1)
			assert.Equal(t, trace.SpanKind(i).String(), singleKindOperations[0].SpanKind)
			assert.Equal(t, operationName, singleKindOperations[0].Name)
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
			cache.Update(serviceName, operationName, trace.SpanKind(i), expireTime)
			assert.Equal(t, expireTime, cache.operations[serviceName][trace.SpanKind(i)][operationName])
		}
	})
}

func TestCacheStore_WriteSpanAndPrefill(t *testing.T) {
	runWithBadger(t, func(store *badger.DB, t *testing.T) {
		cache := NewCacheStore(store, 1*time.Hour, true)
		writer := NewSpanWriter(store, cache, 1*time.Hour)
		for i := 0; i < 6; i++ {
			tid := time.Now()
			testSpan := model.Span{
				TraceID: model.TraceID{
					Low:  uint64(0),
					High: 1,
				},
				SpanID:        model.SpanID(0),
				OperationName: fmt.Sprintf("operation%d", i),
				Process: &model.Process{
					ServiceName: fmt.Sprintf("service%d", i),
				},
				StartTime: tid.Add(1 * time.Millisecond),
				Duration:  1 * time.Millisecond,
				Tags: model.KeyValues{
					model.KeyValue{
						Key:   "span.kind",
						VType: model.StringType,
						VStr:  trace.SpanKind(i).String(),
					},
				},
			}
			err := writer.WriteSpan(context.Background(), &testSpan)
			require.NoError(t, err)
		}
		// The old cache can't be used in assert, as it will not fill anything from store
		// The old cache will get every value from the Update Function called in write span
		// Therefore we will create a new cache and test prefill there
		newCache := NewCacheStore(store, 1*time.Hour, true)
		for i := 0; i < 6; i++ {
			_, foundService := newCache.services[fmt.Sprintf("service%d", i)]
			assert.True(t, foundService)
			_, foundOperation := newCache.operations[fmt.Sprintf("service%d", i)][trace.SpanKind(i)][fmt.Sprintf("operation%d", i)]
			assert.True(t, foundOperation)
		}
	})
}

func TestOldReads(t *testing.T) {
	runWithBadger(t, func(store *badger.DB, t *testing.T) {
		timeNow := model.TimeAsEpochMicroseconds(time.Now())
		s1o1Key := createIndexKey(spanKindIndexKey, []byte("service1operation10080"), timeNow, model.TraceID{High: 0, Low: 0})

		tid := time.Now().Add(1 * time.Minute)

		writer := func() {
			store.Update(func(txn *badger.Txn) error {
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

		cache.Update("service1", "operation1", trace.SpanKindUnspecified, uint64(tid.Unix()))
		cache.services["service1"] = uint64(nuTid.Unix())
		cache.operations["service1"][trace.SpanKindUnspecified]["operation1"] = uint64(nuTid.Unix())

		cache.populateCaches()

		// Now make sure we didn't use the older timestamps from the DB
		assert.Equal(t, uint64(nuTid.Unix()), cache.services["service1"])
		assert.Equal(t, uint64(nuTid.Unix()), cache.operations["service1"][trace.SpanKindUnspecified]["operation1"])
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
