// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"encoding/binary"
	"testing"

	"github.com/dgraph-io/badger/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

func TestGetOperations(t *testing.T) {
	// 1. Setup Badger with a temporary directory (standard pattern)
	opts := badger.DefaultOptions(t.TempDir()).WithLoggingLevel(badger.ERROR)
	db, err := badger.Open(opts)
	require.NoError(t, err)

	// Ensure DB is closed after test
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	reader := NewReader(db)

	// 2. Insert Dummy data
	err = db.Update(func(txn *badger.Txn) error {
		// Helper to create a key in the exact format:
		// [0x82][ServiceName][OperationName][Timestamp(8)][TraceID(16)]
		createKey := func(service, operation string, id uint64) []byte {
			key := make([]byte, 1+len(service)+len(operation)+8+16)
			key[0] = operationNameIndexKey
			n := 1
			n += copy(key[n:], service)
			n += copy(key[n:], operation)

			// Add dummy timestamp (8 bytes)
			binary.BigEndian.PutUint64(key[n:], 123456)
			n += 8

			// Add dummy trace ID (16 bytes)
			binary.BigEndian.PutUint64(key[n:], 0)    // High
			binary.BigEndian.PutUint64(key[n+8:], id) // Low
			return key
		}

		// Add two operations for "service-a" and one for "service-b"
		require.NoError(t, txn.Set(createKey("service-a", "op-1", 1), []byte{}))
		require.NoError(t, txn.Set(createKey("service-a", "op-1", 2), []byte{})) // Duplicate op name, different trace
		require.NoError(t, txn.Set(createKey("service-a", "op-2", 3), []byte{}))
		require.NoError(t, txn.Set(createKey("service-b", "op-3", 4), []byte{}))
		return nil
	})

	require.NoError(t, err)

	// 3. Verify GetOperations for "service-a"
	ops, err := reader.GetOperations(context.Background(),
		tracestore.OperationQueryParams{
			ServiceName: "service-a",
		})
	require.NoError(t, err)

	// We expect 2 unique operations: "op-1" and "op-2"
	assert.Len(t, ops, 2)
	assert.Contains(t, []string{ops[0].Name, ops[1].Name}, "op-1")
	assert.Contains(t, []string{ops[0].Name, ops[1].Name}, "op-2")

	// 4. Verify GetOperations for "service-b"
	ops, err = reader.GetOperations(context.Background(),
		tracestore.OperationQueryParams{
			ServiceName: "service-b",
		})
	require.NoError(t, err)
	assert.Len(t, ops, 1)
	assert.Equal(t, "op-3", ops[0].Name)

	// 5. Verify GetOperations for non-existent service
	ops, err = reader.GetOperations(context.Background(),
		tracestore.OperationQueryParams{
			ServiceName: "service-c",
		})
	require.NoError(t, err)
	assert.Empty(t, ops)
}
