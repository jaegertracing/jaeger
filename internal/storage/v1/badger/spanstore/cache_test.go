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

		cache.Update("service1", "server", "op1", expireTime)
		cache.Update("service1", "client", "op2", expireTime)

		services, err := cache.GetServices()
		require.NoError(t, err)
		assert.Empty(t, services) // Everything should be expired

		// Expired service for operations

		cache.Update("service1", "server", "op1", expireTime)
		cache.Update("service1", "client", "op2", expireTime)

		operations, err := cache.GetOperations(tracestore.OperationQueryParams{ServiceName: "service1"})
		require.NoError(t, err)
		assert.Empty(t, operations) // Everything should be expired

		// Expired operations, stable service

		cache.Update("service1", "server", "op1", expireTime)
		cache.Update("service1", "client", "op2", expireTime)

		cache.services["service1"] = uint64(time.Now().Unix() + 1e10)

		operations, err = cache.GetOperations(tracestore.OperationQueryParams{ServiceName: "service1"})
		require.NoError(t, err)
		assert.Empty(t, operations) // Everything should be expired
	})
}

func TestGetOperationsSpanKindFilter(t *testing.T) {
	runWithBadger(t, func(store *badger.DB, t *testing.T) {
		cache := NewCacheStore(store, time.Duration(1*time.Hour))
		expireTime := uint64(time.Now().Add(cache.ttl).Unix())

		cache.Update("svc", "server", "op1", expireTime)
		cache.Update("svc", "client", "op2", expireTime)
		cache.Update("svc", "", "op3", expireTime) // preloaded without spanKind

		// All operations
		ops, err := cache.GetOperations(tracestore.OperationQueryParams{ServiceName: "svc"})
		require.NoError(t, err)
		assert.Len(t, ops, 3)

		// Filter by server
		ops, err = cache.GetOperations(tracestore.OperationQueryParams{ServiceName: "svc", SpanKind: "server"})
		require.NoError(t, err)
		require.Len(t, ops, 1)
		assert.Equal(t, tracestore.Operation{Name: "op1", SpanKind: "server"}, ops[0])

		// Filter by client
		ops, err = cache.GetOperations(tracestore.OperationQueryParams{ServiceName: "svc", SpanKind: "client"})
		require.NoError(t, err)
		require.Len(t, ops, 1)
		assert.Equal(t, tracestore.Operation{Name: "op2", SpanKind: "client"}, ops[0])

		// Non-existent service
		ops, err = cache.GetOperations(tracestore.OperationQueryParams{ServiceName: "nosvc"})
		require.NoError(t, err)
		assert.Empty(t, ops)
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
