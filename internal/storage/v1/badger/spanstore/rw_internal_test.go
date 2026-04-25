// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"context"
	"encoding/binary"
	"math/rand"
	"testing"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

func TestEncodingTypes(t *testing.T) {
	// JSON encoding
	runWithBadger(t, func(store *badger.DB, t *testing.T) {
		testSpan := createDummySpan()

		cache := NewCacheStore(store, time.Duration(1*time.Hour))
		sw := NewSpanWriter(store, cache, time.Duration(1*time.Hour))
		rw := NewTraceReader(store, cache, true)

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

		cache := NewCacheStore(store, time.Duration(1*time.Hour))
		sw := NewSpanWriter(store, cache, time.Duration(1*time.Hour))
		// rw := NewTraceReader(store, cache)

		sw.encodingType = 0x04
		err := sw.WriteSpan(context.Background(), &testSpan)
		require.EqualError(t, err, "unknown encoding type: 0x04")
	})

	// Unknown encoding reader
	runWithBadger(t, func(store *badger.DB, t *testing.T) {
		testSpan := createDummySpan()

		cache := NewCacheStore(store, time.Duration(1*time.Hour))
		sw := NewSpanWriter(store, cache, time.Duration(1*time.Hour))
		rw := NewTraceReader(store, cache, true)

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
		cache := NewCacheStore(store, time.Duration(1*time.Hour))
		sw := NewSpanWriter(store, cache, time.Duration(1*time.Hour))
		rw := NewTraceReader(store, cache, true)
		origStartTime := testSpan.StartTime

		traceCount := 128
		for range traceCount {
			testSpan.TraceID.Low = rand.Uint64()
			for range 32 {
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

	for i := range 16 {
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

		cache := NewCacheStore(store, time.Duration(-1*time.Hour))
		writer()

		nuTid := tid.Add(1 * time.Hour)

		cache.Update("service1", "operation1", "", uint64(tid.Unix()))
		cache.services["service1"] = uint64(nuTid.Unix())
		cache.operations["service1"][tracestore.Operation{Name: "operation1"}] = uint64(nuTid.Unix())

		// This is equivalent to populate caches of cache
		_ = NewTraceReader(store, cache, true)

		// Now make sure we didn't use the older timestamps from the DB
		assert.Equal(t, uint64(nuTid.Unix()), cache.services["service1"])
		assert.Equal(t, uint64(nuTid.Unix()), cache.operations["service1"][tracestore.Operation{Name: "operation1"}])
	})
}

func TestSpanKindByteMappings(t *testing.T) {
	tests := []struct {
		kind string
		b    byte
	}{
		{"", 0},
		{"internal", 1},
		{"server", 2},
		{"client", 3},
		{"producer", 4},
		{"consumer", 5},
	}
	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			assert.Equal(t, tt.b, spanKindToByte(model.SpanKind(tt.kind)))
			assert.Equal(t, tt.kind, spanKindByteToString(tt.b))
		})
	}
	// Unknown byte maps to empty string
	assert.Empty(t, spanKindByteToString(0xFF))
}

func TestOperationKindIndexKeyLayout(t *testing.T) {
	service := "mysvc"
	operation := "myop"
	var kindByte byte = 2 // server
	startTime := uint64(1000)
	traceID := model.TraceID{High: 1, Low: 2}

	// Build the composite value the same way the writer does
	value := make([]byte, len(service)+1+len(operation))
	copy(value, service)
	value[len(service)] = kindByte
	copy(value[len(service)+1:], operation)

	key := createIndexKey(operationKindIndexKey, value, startTime, traceID)

	// Verify prefix byte
	assert.Equal(t, byte(0x85), key[0])

	// Verify service name
	assert.Equal(t, service, string(key[1:1+len(service)]))

	// Verify kind byte sits between service and operation
	assert.Equal(t, kindByte, key[1+len(service)])

	// Verify operation name
	opStart := 1 + len(service) + 1
	opEnd := len(key) - sizeOfTraceID - 8 // before startTime and traceID
	assert.Equal(t, operation, string(key[opStart:opEnd]))
}

func TestPreloadOperationsWithSpanKind(t *testing.T) {
	runWithBadger(t, func(store *badger.DB, t *testing.T) {
		timeNow := model.TimeAsEpochMicroseconds(time.Now())
		tid := time.Now().Add(1 * time.Minute)

		// Write 0x81 service key
		s1Key := createIndexKey(serviceNameIndexKey, []byte("svc1"), timeNow, model.TraceID{High: 0, Low: 0})

		// Write 0x85 operation+kind keys directly
		serverValue := make([]byte, len("svc1")+1+len("get-users"))
		copy(serverValue, "svc1")
		serverValue[len("svc1")] = 2 // server
		copy(serverValue[len("svc1")+1:], "get-users")
		kindKey1 := createIndexKey(operationKindIndexKey, serverValue, timeNow, model.TraceID{High: 0, Low: 1})

		clientValue := make([]byte, len("svc1")+1+len("db-call"))
		copy(clientValue, "svc1")
		clientValue[len("svc1")] = 3 // client
		copy(clientValue[len("svc1")+1:], "db-call")
		kindKey2 := createIndexKey(operationKindIndexKey, clientValue, timeNow, model.TraceID{High: 0, Low: 2})

		store.Update(func(txn *badger.Txn) error {
			txn.SetEntry(&badger.Entry{Key: s1Key, ExpiresAt: uint64(tid.Unix())})
			txn.SetEntry(&badger.Entry{Key: kindKey1, ExpiresAt: uint64(tid.Unix())})
			txn.SetEntry(&badger.Entry{Key: kindKey2, ExpiresAt: uint64(tid.Unix())})
			return nil
		})

		cache := NewCacheStore(store, time.Duration(1*time.Hour))
		_ = NewTraceReader(store, cache, true)

		// Filter server — preloaded from 0x85 disk keys
		ops, err := cache.GetOperations("svc1", "server")
		require.NoError(t, err)
		assert.Equal(t, []tracestore.Operation{
			{Name: "get-users", SpanKind: "server"},
		}, ops)

		// Filter client — preloaded from 0x85 disk keys
		ops, err = cache.GetOperations("svc1", "client")
		require.NoError(t, err)
		assert.Equal(t, []tracestore.Operation{
			{Name: "db-call", SpanKind: "client"},
		}, ops)

		// All operations
		ops, err = cache.GetOperations("svc1", "")
		require.NoError(t, err)
		assert.ElementsMatch(t, []tracestore.Operation{
			{Name: "get-users", SpanKind: "server"},
			{Name: "db-call", SpanKind: "client"},
		}, ops)
	})
}
