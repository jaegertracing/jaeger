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

		cache.Update("service1", "op1", "server", expireTime)
		cache.Update("service1", "op2", "", expireTime)

		services, err := cache.GetServices()
		require.NoError(t, err)
		assert.Empty(t, services) // Everything should be expired

		// Expired service for operations

		cache.Update("service1", "op1", "server", expireTime)
		cache.Update("service1", "op2", "", expireTime)

		operations, err := cache.GetOperations(tracestore.OperationQueryParams{ServiceName: "service1"})
		require.NoError(t, err)
		assert.Empty(t, operations) // Everything should be expired

		// Expired operations, stable service

		cache.Update("service1", "op1", "server", expireTime)
		cache.Update("service1", "op2", "", expireTime)

		cache.services["service1"] = uint64(time.Now().Unix() + 1e10)

		operations, err = cache.GetOperations(tracestore.OperationQueryParams{ServiceName: "service1"})
		require.NoError(t, err)
		assert.Empty(t, operations) // Everything should be expired
	})
}

func TestSpanKindFiltering(t *testing.T) {
	runWithBadger(t, func(store *badger.DB, t *testing.T) {
		cache := NewCacheStore(store, time.Hour)
		expireTime := uint64(time.Now().Add(cache.ttl).Unix())

		cache.Update("svc", "op1", "server", expireTime)
		cache.Update("svc", "op2", "client", expireTime)
		cache.Update("svc", "op3", "", expireTime)

		all, err := cache.GetOperations(tracestore.OperationQueryParams{ServiceName: "svc"})
		require.NoError(t, err)
		assert.Len(t, all, 3)

		servers, err := cache.GetOperations(tracestore.OperationQueryParams{ServiceName: "svc", SpanKind: "server"})
		require.NoError(t, err)
		require.Len(t, servers, 1)
		assert.Equal(t, "op1", servers[0].Name)
		assert.Equal(t, "server", servers[0].SpanKind)

		clients, err := cache.GetOperations(tracestore.OperationQueryParams{ServiceName: "svc", SpanKind: "client"})
		require.NoError(t, err)
		require.Len(t, clients, 1)
		assert.Equal(t, "op2", clients[0].Name)

		// Empty SpanKind matches all operations regardless of their span kind.
		allViaEmpty, err := cache.GetOperations(tracestore.OperationQueryParams{ServiceName: "svc", SpanKind: ""})
		require.NoError(t, err)
		assert.Len(t, allViaEmpty, 3, "empty SpanKind should return all operations")

		// Non-existent span kind returns empty slice.
		none, err := cache.GetOperations(tracestore.OperationQueryParams{ServiceName: "svc", SpanKind: "producer"})
		require.NoError(t, err)
		assert.Empty(t, none, "unknown SpanKind should return no operations")
	})
}

func TestGetOperationsUnknownService(t *testing.T) {
	runWithBadger(t, func(store *badger.DB, t *testing.T) {
		cache := NewCacheStore(store, time.Hour)

		ops, err := cache.GetOperations(tracestore.OperationQueryParams{ServiceName: "nonexistent"})
		require.NoError(t, err)
		assert.Empty(t, ops, "unknown service should return empty operations slice")
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
