// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package blackhole

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

func withBlackhole(f func(store *Store)) {
	f(NewStore())
}

func TestStoreGetDependencies(t *testing.T) {
	withBlackhole(func(store *Store) {
		links, err := store.GetDependencies(context.Background(), time.Now(), time.Hour)
		require.NoError(t, err)
		assert.Empty(t, links)
	})
}

func TestStoreWriteSpan(t *testing.T) {
	withBlackhole(func(store *Store) {
		err := store.WriteSpan(context.Background(), nil)
		require.NoError(t, err)
	})
}

func TestStoreGetTrace(t *testing.T) {
	withBlackhole(func(store *Store) {
		trace, err := store.GetTrace(context.Background(), spanstore.TraceGetParameters{TraceID: model.NewTraceID(1, 2)})
		require.Error(t, err)
		assert.Nil(t, trace)
	})
}

func TestStoreGetServices(t *testing.T) {
	withBlackhole(func(store *Store) {
		serviceNames, err := store.GetServices(context.Background())
		require.NoError(t, err)
		assert.Empty(t, serviceNames)
	})
}

func TestStoreGetAllOperations(t *testing.T) {
	withBlackhole(func(store *Store) {
		operations, err := store.GetOperations(
			context.Background(),
			spanstore.OperationQueryParameters{},
		)
		require.NoError(t, err)
		assert.Empty(t, operations)
	})
}

func TestStoreFindTraces(t *testing.T) {
	withBlackhole(func(store *Store) {
		traces, err := store.FindTraces(context.Background(), nil)
		require.NoError(t, err)
		assert.Empty(t, traces)
	})
}

func TestStoreFindTraceIDs(t *testing.T) {
	withBlackhole(func(store *Store) {
		traceIDs, err := store.FindTraceIDs(context.Background(), nil)
		require.NoError(t, err)
		assert.Empty(t, traceIDs)
	})
}
