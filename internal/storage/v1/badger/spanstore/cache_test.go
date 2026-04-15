// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"testing"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

/*
	Additional cache store tests that need to access internal parts. As such, package must be spanstore and not spanstore_test
*/

func TestExpiredItems(t *testing.T) {
	runWithBadger(t, func(store *badger.DB, t *testing.T) {
		cache := NewCacheStore(store, time.Duration(-1*time.Hour))

		expireTime := uint64(time.Now().Add(cache.ttl).Unix())

		// Expired service

		cache.Update("service1", "op1", "", expireTime)
		cache.Update("service1", "op2", "", expireTime)

		services, err := cache.GetServices()
		require.NoError(t, err)
		assert.Empty(t, services) // Everything should be expired

		// Expired service for operations

		cache.Update("service1", "op1", "", expireTime)
		cache.Update("service1", "op2", "", expireTime)

		operations, err := cache.GetOperations("service1", "")
		require.NoError(t, err)
		assert.Empty(t, operations) // Everything should be expired

		// Expired operations, stable service

		cache.Update("service1", "op1", "", expireTime)
		cache.Update("service1", "op2", "", expireTime)

		cache.services["service1"] = uint64(time.Now().Unix() + 1e10)

		operations, err = cache.GetOperations("service1", "")
		require.NoError(t, err)
		assert.Empty(t, operations) // Everything should be expired
	})
}

func TestSpanKindFilter(t *testing.T) {
	runWithBadger(t, func(store *badger.DB, t *testing.T) {
		cache := NewCacheStore(store, time.Duration(1*time.Hour))

		expireTime := uint64(time.Now().Add(1 * time.Hour).Unix())

		cache.Update("svc", "op-a", "server", expireTime)
		cache.Update("svc", "op-b", "client", expireTime)
		cache.Update("svc", "op-a", "client", expireTime)
		cache.Update("svc", "op-c", "", expireTime)

		// Empty spanKind returns all
		ops, err := cache.GetOperations("svc", "")
		require.NoError(t, err)
		assert.ElementsMatch(t, []tracestore.Operation{
			{Name: "op-a", SpanKind: "server"},
			{Name: "op-b", SpanKind: "client"},
			{Name: "op-a", SpanKind: "client"},
			{Name: "op-c", SpanKind: ""},
		}, ops)

		// Filter server
		ops, err = cache.GetOperations("svc", "server")
		require.NoError(t, err)
		assert.Equal(t, []tracestore.Operation{
			{Name: "op-a", SpanKind: "server"},
		}, ops)

		// Filter client
		ops, err = cache.GetOperations("svc", "client")
		require.NoError(t, err)
		assert.ElementsMatch(t, []tracestore.Operation{
			{Name: "op-a", SpanKind: "client"},
			{Name: "op-b", SpanKind: "client"},
		}, ops)

		// Filter with no matches
		ops, err = cache.GetOperations("svc", "producer")
		require.NoError(t, err)
		assert.Empty(t, ops)
	})
}

func TestAddOperationKeepsLaterTTL(t *testing.T) {
	runWithBadger(t, func(store *badger.DB, t *testing.T) {
		cache := NewCacheStore(store, time.Duration(1*time.Hour))

		later := uint64(time.Now().Add(2 * time.Hour).Unix())
		earlier := uint64(time.Now().Add(1 * time.Hour).Unix())

		// First add with later TTL
		cache.AddOperation("svc", "op", "server", later)
		// Second add with earlier TTL — should be ignored
		cache.AddOperation("svc", "op", "server", earlier)

		// Force service alive so cache returns operations
		cache.services["svc"] = later

		ops, err := cache.GetOperations("svc", "server")
		require.NoError(t, err)
		assert.Equal(t, []tracestore.Operation{
			{Name: "op", SpanKind: "server"},
		}, ops)
		// Verify internal map has the later TTL (not overwritten)
		assert.Equal(t, later, cache.operations["svc"][tracestore.Operation{Name: "op", SpanKind: "server"}])
	})
}

func TestSpanKindExpiry(t *testing.T) {
	runWithBadger(t, func(store *badger.DB, t *testing.T) {
		cache := NewCacheStore(store, time.Duration(1*time.Hour))

		fresh := uint64(time.Now().Add(1 * time.Hour).Unix())
		stale := uint64(time.Now().Add(-1 * time.Hour).Unix())

		cache.Update("svc", "op-a", "server", fresh)
		cache.Update("svc", "op-b", "client", stale)
		cache.services["svc"] = fresh // keep service alive

		ops, err := cache.GetOperations("svc", "")
		require.NoError(t, err)
		assert.Equal(t, []tracestore.Operation{
			{Name: "op-a", SpanKind: "server"},
		}, ops)
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
