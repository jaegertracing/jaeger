// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"context"
	"encoding/binary"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
)

func TestEncodingTypes(t *testing.T) {
	// JSON encoding
	runWithBadger(t, func(store *badger.DB, t *testing.T) {
		testSpan := createDummySpan()

		cache := NewCacheStore(time.Duration(1 * time.Hour))
		sw := NewSpanWriter(store, cache, time.Duration(1*time.Hour))
		rw := NewTraceReader(store, cache, true, true)

		sw.encodingType = jsonEncoding
		err := sw.WriteSpan(context.Background(), &testSpan)
		require.NoError(t, err)

		tr, err := rw.GetTrace(context.Background(), spanstore.GetTraceParameters{TraceID: model.TraceID{Low: 0, High: 1}})
		require.NoError(t, err)
		assert.Len(t, tr.Spans, 1)
	})

	// Unknown encoding write
	runWithBadger(t, func(store *badger.DB, t *testing.T) {
		testSpan := createDummySpan()

		cache := NewCacheStore(time.Duration(1 * time.Hour))
		sw := NewSpanWriter(store, cache, time.Duration(1*time.Hour))
		// rw := NewTraceReader(store, cache)

		sw.encodingType = 0x04
		err := sw.WriteSpan(context.Background(), &testSpan)
		require.EqualError(t, err, "unknown encoding type: 0x04")
	})

	// Unknown encoding reader
	runWithBadger(t, func(store *badger.DB, t *testing.T) {
		testSpan := createDummySpan()

		cache := NewCacheStore(time.Duration(1 * time.Hour))
		sw := NewSpanWriter(store, cache, time.Duration(1*time.Hour))
		rw := NewTraceReader(store, cache, true, true)

		err := sw.WriteSpan(context.Background(), &testSpan)
		require.NoError(t, err)

		startTime := model.TimeAsEpochMicroseconds(testSpan.StartTime)

		key, _, _ := createTraceKV(&testSpan, protoEncoding, startTime)
		e := &badger.Entry{
			Key:       key,
			ExpiresAt: uint64(time.Now().Add(1 * time.Hour).Unix()),
		}
		e.UserMeta = byte(0x04)

		store.Update(func(txn *badger.Txn) error {
			txn.SetEntry(e)
			return nil
		})

		_, err = rw.GetTrace(context.Background(), spanstore.GetTraceParameters{TraceID: model.TraceID{Low: 0, High: 1}})
		require.EqualError(t, err, "unknown encoding type: 0x04")
	})
}

func TestDecodeErrorReturns(t *testing.T) {
	garbage := []byte{0x08}

	_, err := decodeValue(garbage, protoEncoding)
	require.Error(t, err)

	_, err = decodeValue(garbage, jsonEncoding)
	require.Error(t, err)
}

func TestDuplicateTraceIDDetection(t *testing.T) {
	runWithBadger(t, func(store *badger.DB, t *testing.T) {
		testSpan := createDummySpan()
		cache := NewCacheStore(time.Duration(1 * time.Hour))
		sw := NewSpanWriter(store, cache, time.Duration(1*time.Hour))
		rw := NewTraceReader(store, cache, true, true)
		origStartTime := testSpan.StartTime

		traceCount := 128
		for k := 0; k < traceCount; k++ {
			testSpan.TraceID.Low = rand.Uint64()
			for i := 0; i < 32; i++ {
				testSpan.SpanID = model.SpanID(rand.Uint64())
				testSpan.StartTime = origStartTime.Add(time.Duration(rand.Int31n(8000)) * time.Millisecond)
				err := sw.WriteSpan(context.Background(), &testSpan)
				require.NoError(t, err)
			}
		}

		traces, err := rw.FindTraceIDs(context.Background(), &spanstore.TraceQueryParameters{
			ServiceName:  "service",
			NumTraces:    256, // Default is 100, we want to fetch more than there should be
			StartTimeMax: time.Now().Add(time.Hour),
			StartTimeMin: testSpan.StartTime.Add(-1 * time.Hour),
		})

		require.NoError(t, err)
		assert.Len(t, traces, 128)
	})
}

func createDummySpan() model.Span {
	tid := time.Now()

	dummyKv := []model.KeyValue{
		{
			Key:   "key",
			VType: model.StringType,
			VStr:  "value",
		},
	}

	testSpan := model.Span{
		TraceID: model.TraceID{
			Low:  uint64(0),
			High: 1,
		},
		SpanID:        model.SpanID(0),
		OperationName: "operation",
		Process: &model.Process{
			ServiceName: "service",
			Tags:        dummyKv,
		},
		StartTime: tid.Add(time.Duration(1 * time.Millisecond)),
		Duration:  time.Duration(1 * time.Millisecond),
		Tags:      dummyKv,
		Logs: []model.Log{
			{
				Timestamp: tid,
				Fields:    dummyKv,
			},
		},
	}

	return testSpan
}

func TestMergeJoin(t *testing.T) {
	chk := assert.New(t)

	// Test equals

	left := make([][]byte, 16)
	right := make([][]byte, 16)

	for i := 0; i < 16; i++ {
		left[i] = make([]byte, 4)
		binary.BigEndian.PutUint32(left[i], uint32(i))

		right[i] = make([]byte, 4)
		binary.BigEndian.PutUint32(right[i], uint32(i))
	}

	merged := mergeJoinIds(left, right)
	chk.Len(merged, 16)

	// Check order
	chk.Equal(uint32(15), binary.BigEndian.Uint32(merged[15]))

	// Test simple non-equality different size

	merged = mergeJoinIds(left[1:2], right[13:])
	chk.Empty(merged)

	// Different size, some equalities

	merged = mergeJoinIds(left[0:3], right[1:7])
	chk.Len(merged, 2)
	chk.Equal(uint32(2), binary.BigEndian.Uint32(merged[1]))
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

		cache := NewCacheStore(time.Duration(-1 * time.Hour))
		writer()

		nuTid := tid.Add(1 * time.Hour)

		cache.Update("service1", "operation1", model.SpanKindUnspecified, uint64(tid.Unix()))
		cache.services["service1"] = uint64(nuTid.Unix())
		cache.operations["service1"][model.SpanKindUnspecified]["operation1"] = uint64(nuTid.Unix())

		// This is equivalent to populate caches of cache
		_ = NewTraceReader(store, cache, true, true)

		// Now make sure we didn't use the older timestamps from the DB
		assert.Equal(t, uint64(nuTid.Unix()), cache.services["service1"])
		assert.Equal(t, uint64(nuTid.Unix()), cache.operations["service1"][model.SpanKindUnspecified]["operation1"])
	})
}

// Code Coverage Test
func TestCacheStore_WrongSpanKindFromBadger(t *testing.T) {
	runWithBadger(t, func(store *badger.DB, t *testing.T) {
		cache := NewCacheStore(1 * time.Hour)
		writer := NewSpanWriter(store, cache, 1*time.Hour)
		_ = NewTraceReader(store, cache, true, true)
		span := createDummySpanWithKind("service", "operation", model.SpanKind("New Kind"), true)
		err := writer.WriteSpan(context.Background(), span)
		require.NoError(t, err)
		newCache := NewCacheStore(1 * time.Hour)
		_ = NewTraceReader(store, newCache, true, true)
		services, err := newCache.GetServices()
		require.NoError(t, err)
		assert.Len(t, services, 1)
		assert.Equal(t, "service", services[0])
		operations, err := newCache.GetOperations("service", "")
		require.NoError(t, err)
		assert.Len(t, operations, 1)
		assert.Equal(t, "", operations[0].SpanKind)
		assert.Equal(t, "operation", operations[0].Name)
	})
}

// Test Case for old data
func TestCacheStore_WhenValueIsNil(t *testing.T) {
	runWithBadger(t, func(store *badger.DB, t *testing.T) {
		cache := NewCacheStore(1 * time.Hour)
		w := NewSpanWriter(store, cache, 1*time.Hour)
		_ = NewTraceReader(store, cache, true, true)
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
		newCache := NewCacheStore(1 * time.Hour)
		_ = NewTraceReader(store, newCache, true, true)
		services, err := newCache.GetServices()
		require.NoError(t, err)
		assert.Len(t, services, 1)
		assert.Equal(t, "service", services[0])
		operations, err := newCache.GetOperations("service", "")
		require.NoError(t, err)
		assert.Len(t, operations, 1)
		assert.Equal(t, "", operations[0].SpanKind)
		assert.Equal(t, "operation", operations[0].Name)
	})
}

func TestCacheStore_Prefill(t *testing.T) {
	runWithBadger(t, func(store *badger.DB, t *testing.T) {
		cache := NewCacheStore(1 * time.Hour)
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
		newCache := NewCacheStore(1 * time.Hour)
		_ = NewTraceReader(store, newCache, true, true)
		for i, span := range spans {
			_, foundService := newCache.services[span.Process.ServiceName]
			assert.True(t, foundService)
			_, foundOperation := newCache.operations[span.Process.ServiceName][spanKinds[i]][span.OperationName]
			assert.True(t, foundOperation)
		}
	})
}
