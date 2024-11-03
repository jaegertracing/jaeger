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

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

func TestEncodingTypes(t *testing.T) {
	// JSON encoding
	runWithBadger(t, func(store *badger.DB, t *testing.T) {
		testSpan := createDummySpan()

		cache := NewCacheStore(store, time.Duration(1*time.Hour), true)
		sw := NewSpanWriter(store, cache, time.Duration(1*time.Hour))
		rw := NewTraceReader(store, cache)

		sw.encodingType = jsonEncoding
		err := sw.WriteSpan(context.Background(), &testSpan)
		require.NoError(t, err)

		tr, err := rw.GetTrace(context.Background(), spanstore.TraceGetParameters{TraceID: model.TraceID{Low: 0, High: 1}})
		require.NoError(t, err)
		assert.Len(t, tr.Spans, 1)
	})

	// Unknown encoding write
	runWithBadger(t, func(store *badger.DB, t *testing.T) {
		testSpan := createDummySpan()

		cache := NewCacheStore(store, time.Duration(1*time.Hour), true)
		sw := NewSpanWriter(store, cache, time.Duration(1*time.Hour))
		// rw := NewTraceReader(store, cache)

		sw.encodingType = 0x04
		err := sw.WriteSpan(context.Background(), &testSpan)
		require.EqualError(t, err, "unknown encoding type: 0x04")
	})

	// Unknown encoding reader
	runWithBadger(t, func(store *badger.DB, t *testing.T) {
		testSpan := createDummySpan()

		cache := NewCacheStore(store, time.Duration(1*time.Hour), true)
		sw := NewSpanWriter(store, cache, time.Duration(1*time.Hour))
		rw := NewTraceReader(store, cache)

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

		_, err = rw.GetTrace(context.Background(), spanstore.TraceGetParameters{TraceID: model.TraceID{Low: 0, High: 1}})
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
		cache := NewCacheStore(store, time.Duration(1*time.Hour), true)
		sw := NewSpanWriter(store, cache, time.Duration(1*time.Hour))
		rw := NewTraceReader(store, cache)
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
