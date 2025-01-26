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

	"github.com/jaegertracing/jaeger-idl/model/v1"
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
	runWithBadger(t, func(_ *badger.DB, t *testing.T) {
		cache := NewCacheStore(time.Duration(-1 * time.Hour))

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

func TestCacheStore_GetOperations(t *testing.T) {
	runWithBadger(t, func(_ *badger.DB, t *testing.T) {
		cache := NewCacheStore(1 * time.Hour)
		expireTime := uint64(time.Now().Add(cache.ttl).Unix())
		serviceName := "service1"
		operationName := "op1"
		for i := 0; i <= 5; i++ {
			cache.Update(serviceName, operationName, spanKinds[i], expireTime)
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
	runWithBadger(t, func(_ *badger.DB, t *testing.T) {
		cache := NewCacheStore(1 * time.Hour)
		expireTime := uint64(time.Now().Add(cache.ttl).Unix())
		serviceName := "service1"
		operationName := "op1"
		for i := 0; i <= 5; i++ {
			cache.Update(serviceName, operationName, spanKinds[i], expireTime)
			assert.Equal(t, expireTime, cache.operations[serviceName][spanKinds[i]][operationName])
		}
	})
}

func TestCacheStore_GetOperationsReturnsEmptyOperationsWithWrongSpanKind(t *testing.T) {
	runWithBadger(t, func(_ *badger.DB, t *testing.T) {
		cache := NewCacheStore(1 * time.Hour)
		cache.Update("service", "operation", model.SpanKindUnspecified, uint64(time.Now().Unix()))
		operations, err := cache.GetOperations("service1", "a")
		require.NoError(t, err)
		assert.Empty(t, operations)
	})
}

func TestCacheStore_GetOperationsSameKind(t *testing.T) {
	runWithBadger(t, func(store *badger.DB, t *testing.T) {
		cache := NewCacheStore(1 * time.Hour)
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
