// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"encoding/binary"
	"testing"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	conventions "github.com/jaegertracing/jaeger/internal/telemetry/otelsemconv"
)

func setupBadger(t *testing.T) *badger.DB {
	opts := badger.DefaultOptions(t.TempDir())
	opts.SyncWrites = false
	db, err := badger.Open(opts)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

func TestTraceWriter_WriteTraces(t *testing.T) {
	db := setupBadger(t)
	writer := NewTraceWriter(db, time.Hour)
	td := makeTraces()

	err := writer.WriteTraces(context.Background(), td)
	require.NoError(t, err)

	// Verify span was written
	var spanCount int
	err = db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()

		prefix := []byte{spanKeyPrefix}
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			spanCount++

			// Verify we can unmarshal the value
			item := it.Item()
			val, err := item.ValueCopy(nil)
			require.NoError(t, err)

			unmarshaler := &ptrace.ProtoUnmarshaler{}
			traces, err := unmarshaler.UnmarshalTraces(val)
			require.NoError(t, err)
			assert.Equal(t, 1, traces.ResourceSpans().Len())
		}
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 1, spanCount)
}

func TestTraceWriter_WriteTraces_ServiceIndex(t *testing.T) {
	db := setupBadger(t)
	writer := NewTraceWriter(db, time.Hour)
	td := makeTraces()

	err := writer.WriteTraces(context.Background(), td)
	require.NoError(t, err)

	// Verify service index was created
	var indexFound bool
	err = db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()

		prefix := []byte{serviceNameIndexKey}
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			indexFound = true
		}
		return nil
	})
	require.NoError(t, err)
	assert.True(t, indexFound)
}

func TestTraceWriter_WriteTraces_OperationIndex(t *testing.T) {
	db := setupBadger(t)
	writer := NewTraceWriter(db, time.Hour)
	td := makeTraces()

	err := writer.WriteTraces(context.Background(), td)
	require.NoError(t, err)

	// Verify operation index was created
	var indexFound bool
	err = db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()

		prefix := []byte{operationNameIndexKey}
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			indexFound = true
		}
		return nil
	})
	require.NoError(t, err)
	assert.True(t, indexFound)
}

func TestTraceWriter_WriteTraces_MultipleSpans(t *testing.T) {
	db := setupBadger(t)
	writer := NewTraceWriter(db, time.Hour)
	td := makeTracesWithMultipleSpans(3)

	err := writer.WriteTraces(context.Background(), td)
	require.NoError(t, err)

	// Count spans
	var spanCount int
	err = db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()

		prefix := []byte{spanKeyPrefix}
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			spanCount++
		}
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 3, spanCount)
}

func TestTraceWriter_WriteTraces_EmptyTraces(t *testing.T) {
	db := setupBadger(t)
	writer := NewTraceWriter(db, time.Hour)
	td := ptrace.NewTraces()

	err := writer.WriteTraces(context.Background(), td)
	require.NoError(t, err)
}

func TestTraceWriter_WriteTraces_NoServiceName(t *testing.T) {
	db := setupBadger(t)
	writer := NewTraceWriter(db, time.Hour)

	// Create traces without service name
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()
	span.SetName("test-op")
	span.SetTraceID(traceIDFromInts(0, 1))
	span.SetSpanID(spanIDFromInt(1))

	err := writer.WriteTraces(context.Background(), td)
	require.NoError(t, err)

	// Span should be written
	var spanFound bool
	err = db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()

		prefix := []byte{spanKeyPrefix}
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			spanFound = true
		}
		return nil
	})
	require.NoError(t, err)
	assert.True(t, spanFound)

	// But service index should NOT be created
	var serviceIndexFound bool
	err = db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()

		prefix := []byte{serviceNameIndexKey}
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			serviceIndexFound = true
		}
		return nil
	})
	require.NoError(t, err)
	assert.False(t, serviceIndexFound)
}

func TestTraceWriter_WriteTraces_TagIndexes(t *testing.T) {
	db := setupBadger(t)
	writer := NewTraceWriter(db, time.Hour)
	td := makeTracesWithAttributes()

	err := writer.WriteTraces(context.Background(), td)
	require.NoError(t, err)

	// Verify tag indexes were created
	var tagIndexCount int
	err = db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()

		prefix := []byte{tagIndexKey}
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			tagIndexCount++
		}
		return nil
	})
	require.NoError(t, err)
	assert.Positive(t, tagIndexCount)
}

func TestTraceWriter_WriteTraces_DurationIndex(t *testing.T) {
	db := setupBadger(t)
	writer := NewTraceWriter(db, time.Hour)
	td := makeTraces()

	err := writer.WriteTraces(context.Background(), td)
	require.NoError(t, err)

	// Verify duration index was created
	var durationIndexFound bool
	err = db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()

		prefix := []byte{durationIndexKey}
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			durationIndexFound = true
		}
		return nil
	})
	require.NoError(t, err)
	assert.True(t, durationIndexFound)
}

func TestNewTraceWriter(t *testing.T) {
	db := setupBadger(t)
	writer := NewTraceWriter(db, time.Hour)
	assert.NotNil(t, writer)
	assert.Equal(t, db, writer.store)
	assert.Equal(t, time.Hour, writer.ttl)
}

func TestCreateSpanKey(t *testing.T) {
	traceID := traceIDFromInts(0x0102030405060708, 0x090a0b0c0d0e0f10)
	spanID := spanIDFromInt(0x1112131415161718)
	startTime := uint64(1234567890)

	key := createSpanKey(traceID, startTime, spanID)

	assert.Equal(t, spanKeyPrefix, key[0])
	assert.Len(t, key, 1+sizeOfTraceID+8+sizeOfSpanID)
	assert.Equal(t, traceID[:], key[1:1+sizeOfTraceID])
	gotTime := binary.BigEndian.Uint64(key[1+sizeOfTraceID : 1+sizeOfTraceID+8])
	assert.Equal(t, startTime, gotTime)
	assert.Equal(t, spanID[:], key[1+sizeOfTraceID+8:])
}

func TestCreateIndexKey(t *testing.T) {
	traceID := traceIDFromInts(0x0102030405060708, 0x090a0b0c0d0e0f10)
	startTime := uint64(1234567890)
	value := []byte("test-value")

	key := createIndexKey(serviceNameIndexKey, value, startTime, traceID)

	assert.Equal(t, (serviceNameIndexKey&indexKeyRange)|spanKeyPrefix, key[0])
	assert.Len(t, key, 1+len(value)+8+sizeOfTraceID)
	assert.Equal(t, value, key[1:1+len(value)])
	gotTime := binary.BigEndian.Uint64(key[1+len(value) : 1+len(value)+8])
	assert.Equal(t, startTime, gotTime)
	assert.Equal(t, traceID[:], key[1+len(value)+8:])
}

func TestSeparatorPreventsKeyCollision(t *testing.T) {
	// This test verifies that the separator prevents key collisions
	// e.g., service="foo" + op="bar" should differ from service="foob" + op="ar"
	traceID := traceIDFromInts(0, 1)
	startTime := uint64(1234567890)

	// Without separator, "foo"+"bar" == "foob"+"ar" == "foobar"
	// With separator, "foo\x00bar" != "foob\x00ar"
	key1 := createIndexKey(operationNameIndexKey, []byte("foo"+separator+"bar"), startTime, traceID)
	key2 := createIndexKey(operationNameIndexKey, []byte("foob"+separator+"ar"), startTime, traceID)

	assert.NotEqual(t, key1, key2, "keys should differ due to separator")

	// Also verify tag keys with multiple separators
	tagKey1 := createIndexKey(tagIndexKey, []byte("svc"+separator+"key"+separator+"val"), startTime, traceID)
	tagKey2 := createIndexKey(tagIndexKey, []byte("svck"+separator+"ey"+separator+"val"), startTime, traceID)

	assert.NotEqual(t, tagKey1, tagKey2, "tag keys should differ due to separators")
}

func TestMarshalSpan(t *testing.T) {
	td := makeTraces()
	rs := td.ResourceSpans().At(0)
	ss := rs.ScopeSpans().At(0)
	span := ss.Spans().At(0)

	data, err := marshalSpan(rs, ss, span)
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// Unmarshal and verify
	unmarshaler := &ptrace.ProtoUnmarshaler{}
	traces, err := unmarshaler.UnmarshalTraces(data)
	require.NoError(t, err)
	assert.Equal(t, 1, traces.ResourceSpans().Len())

	gotRS := traces.ResourceSpans().At(0)
	svcName, ok := gotRS.Resource().Attributes().Get(conventions.ServiceNameKey)
	assert.True(t, ok)
	assert.Equal(t, "test-service", svcName.Str())
}

func TestGetServiceName(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() pcommon.Resource
		expected string
	}{
		{
			name: "service name present",
			setup: func() pcommon.Resource {
				td := ptrace.NewTraces()
				rs := td.ResourceSpans().AppendEmpty()
				rs.Resource().Attributes().PutStr(conventions.ServiceNameKey, "my-service")
				return rs.Resource()
			},
			expected: "my-service",
		},
		{
			name: "service name missing",
			setup: func() pcommon.Resource {
				td := ptrace.NewTraces()
				rs := td.ResourceSpans().AppendEmpty()
				return rs.Resource()
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource := tt.setup()
			result := getServiceName(resource)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Helper functions

func traceIDFromInts(high, low uint64) pcommon.TraceID {
	var id pcommon.TraceID
	binary.BigEndian.PutUint64(id[:8], high)
	binary.BigEndian.PutUint64(id[8:], low)
	return id
}

func spanIDFromInt(val uint64) pcommon.SpanID {
	var id pcommon.SpanID
	binary.BigEndian.PutUint64(id[:], val)
	return id
}

func makeTraces() ptrace.Traces {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr(conventions.ServiceNameKey, "test-service")

	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()
	span.SetName("test-operation")
	span.SetTraceID(traceIDFromInts(0, 1))
	span.SetSpanID(spanIDFromInt(1))
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Now()))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(time.Now().Add(100 * time.Millisecond)))

	return td
}

func makeTracesWithMultipleSpans(count int) ptrace.Traces {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr(conventions.ServiceNameKey, "test-service")

	ss := rs.ScopeSpans().AppendEmpty()
	for i := 0; i < count; i++ {
		span := ss.Spans().AppendEmpty()
		span.SetName("test-operation")
		span.SetTraceID(traceIDFromInts(0, 1))
		span.SetSpanID(spanIDFromInt(uint64(i + 1)))
		span.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Now()))
		span.SetEndTimestamp(pcommon.NewTimestampFromTime(time.Now().Add(100 * time.Millisecond)))
	}

	return td
}

func makeTracesWithAttributes() ptrace.Traces {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr(conventions.ServiceNameKey, "test-service")
	rs.Resource().Attributes().PutStr("deployment.environment", "test")

	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()
	span.SetName("test-operation")
	span.SetTraceID(traceIDFromInts(0, 1))
	span.SetSpanID(spanIDFromInt(1))
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Now()))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(time.Now().Add(100 * time.Millisecond)))
	span.Attributes().PutStr("http.method", "GET")
	span.Attributes().PutInt("http.status_code", 200)

	return td
}

func TestTraceWriter_WriteTraces_Error_ClosedDB(t *testing.T) {
	db := setupBadger(t)
	writer := NewTraceWriter(db, time.Hour)
	td := makeTraces()

	// Close the DB to force an error
	require.NoError(t, db.Close())

	err := writer.WriteTraces(context.Background(), td)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DB Closed")
}

func TestTraceWriter_WriteTraces_Error_KeyTooLarge(t *testing.T) {
	db := setupBadger(t)
	writer := NewTraceWriter(db, time.Hour)
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	// Create a huge service name > 65KB (Badger's key limit)
	hugeService := string(make([]byte, 70000))
	rs.Resource().Attributes().PutStr(conventions.ServiceNameKey, hugeService)

	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()
	span.SetName("test-operation")
	span.SetTraceID(traceIDFromInts(0, 1))
	span.SetSpanID(spanIDFromInt(1))
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Now()))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(time.Now().Add(100 * time.Millisecond)))

	err := writer.WriteTraces(context.Background(), td)
	require.Error(t, err)
	// Badger error for key too large
	assert.Contains(t, err.Error(), "Key with size")
	assert.Contains(t, err.Error(), "exceeded")
}

func TestTraceWriter_WriteTraces_Error_OperationKeyTooLarge(t *testing.T) {
	db := setupBadger(t)
	writer := NewTraceWriter(db, time.Hour)
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr(conventions.ServiceNameKey, "normal-service")

	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()
	// Huge operation name to fail operation index write
	hugeOp := string(make([]byte, 70000))
	span.SetName(hugeOp)
	span.SetTraceID(traceIDFromInts(0, 1))
	span.SetSpanID(spanIDFromInt(1))
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Now()))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(time.Now().Add(100 * time.Millisecond)))

	err := writer.WriteTraces(context.Background(), td)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Key with size")
}

func TestTraceWriter_WriteTraces_Error_TagKeyTooLarge(t *testing.T) {
	db := setupBadger(t)
	writer := NewTraceWriter(db, time.Hour)
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr(conventions.ServiceNameKey, "normal-service")

	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()
	span.SetName("normal-operation")
	span.SetTraceID(traceIDFromInts(0, 1))
	span.SetSpanID(spanIDFromInt(1))
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Now()))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(time.Now().Add(100 * time.Millisecond)))

	// Huge attribute value to fail tag index write
	hugeVal := string(make([]byte, 70000))
	span.Attributes().PutStr("regular-key", hugeVal)

	err := writer.WriteTraces(context.Background(), td)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Key with size")
}

func TestTraceWriter_WriteTraces_Error_ResourceTagKeyTooLarge(t *testing.T) {
	db := setupBadger(t)
	writer := NewTraceWriter(db, time.Hour)
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	// Huge resource attribute
	hugeVal := string(make([]byte, 70000))
	rs.Resource().Attributes().PutStr("huge-resource-attr", hugeVal)
	rs.Resource().Attributes().PutStr(conventions.ServiceNameKey, "normal-service")

	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()
	span.SetName("normal-operation")
	span.SetTraceID(traceIDFromInts(0, 1))
	span.SetSpanID(spanIDFromInt(1))
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Now()))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(time.Now().Add(100 * time.Millisecond)))

	err := writer.WriteTraces(context.Background(), td)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Key with size")
}

func TestTraceWriter_WriteTraces_Error_EventTagKeyTooLarge(t *testing.T) {
	db := setupBadger(t)
	writer := NewTraceWriter(db, time.Hour)
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr(conventions.ServiceNameKey, "normal-service")

	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()
	span.SetName("normal-operation")
	span.SetTraceID(traceIDFromInts(0, 1))
	span.SetSpanID(spanIDFromInt(1))
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Now()))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(time.Now().Add(100 * time.Millisecond)))

	// Huge event attribute
	event := span.Events().AppendEmpty()
	event.SetName("test-event")
	hugeVal := string(make([]byte, 70000))
	event.Attributes().PutStr("huge-event-attr", hugeVal)

	err := writer.WriteTraces(context.Background(), td)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Key with size")
}

func TestTraceWriter_WriteTraces_DurationUnderflow(t *testing.T) {
	db := setupBadger(t)
	writer := NewTraceWriter(db, time.Hour)
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr(conventions.ServiceNameKey, "test-service")

	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()
	span.SetName("test-operation")
	span.SetTraceID(traceIDFromInts(0, 1))
	span.SetSpanID(spanIDFromInt(1))

	// Start time is AFTER end time, which should cause underflow if not handled
	now := time.Now()
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(now.Add(time.Hour)))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(now))

	err := writer.WriteTraces(context.Background(), td)
	require.NoError(t, err)

	// Verify duration index
	err = db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		// Decode the key to check duration
		// durationIndexKey format: <prefix><duration><startTime><traceId>
		it := txn.NewIterator(opts)
		defer it.Close()

		prefix := []byte{durationIndexKey}
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			key := it.Item().Key()
			// Prefix (1) + Duration (8)
			if len(key) < 9 {
				continue
			}
			durationBytes := key[1:9]
			duration := binary.BigEndian.Uint64(durationBytes)

			// If underflow occurred, duration will be huge (near max uint64)
			// If fixed, it should be 0
			assert.Equal(t, uint64(0), duration, "Duration should be 0 for invalid span timestamps")
		}
		return nil
	})
	require.NoError(t, err)
}
