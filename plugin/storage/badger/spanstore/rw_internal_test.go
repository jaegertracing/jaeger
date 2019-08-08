// Copyright (c) 2019 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package spanstore

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

func TestEncodingTypes(t *testing.T) {
	// JSON encoding
	runWithBadger(t, func(store *badger.DB, t *testing.T) {
		testSpan := createDummySpan()

		cache := NewCacheStore(store, time.Duration(1*time.Hour), true)
		sw := NewSpanWriter(store, cache, time.Duration(1*time.Hour), nil)
		rw := NewTraceReader(store, cache)

		sw.encodingType = jsonEncoding
		err := sw.WriteSpan(&testSpan)
		assert.NoError(t, err)

		tr, err := rw.GetTrace(context.Background(), model.TraceID{Low: 0, High: 1})
		assert.NoError(t, err)
		assert.Equal(t, 1, len(tr.Spans))
	})

	// Unknown encoding write
	runWithBadger(t, func(store *badger.DB, t *testing.T) {
		testSpan := createDummySpan()

		cache := NewCacheStore(store, time.Duration(1*time.Hour), true)
		sw := NewSpanWriter(store, cache, time.Duration(1*time.Hour), nil)

		sw.encodingType = 0x04
		err := sw.WriteSpan(&testSpan)
		assert.EqualError(t, err, "unknown encoding type: 0x04")
	})

	// Unknown encoding reader
	runWithBadger(t, func(store *badger.DB, t *testing.T) {
		testSpan := createDummySpan()

		cache := NewCacheStore(store, time.Duration(1*time.Hour), true)
		sw := NewSpanWriter(store, cache, time.Duration(1*time.Hour), nil)
		rw := NewTraceReader(store, cache)

		err := sw.WriteSpan(&testSpan)
		assert.NoError(t, err)

		key, _, _ := createTraceKV(&testSpan, protoEncoding)
		e := &badger.Entry{
			Key:       key,
			ExpiresAt: uint64(time.Now().Add(1 * time.Hour).Unix()),
		}
		e.UserMeta = byte(0x04)

		store.Update(func(txn *badger.Txn) error {
			txn.SetEntry(e)
			return nil
		})

		_, err = rw.GetTrace(context.Background(), model.TraceID{Low: 0, High: 1})
		assert.EqualError(t, err, "unknown encoding type: 0x04")
	})
}

func TestScanDependencyIndexKey(t *testing.T) {
	// This functionality is tested through dependencystore's storage_test.go, but the codecov
	// can't detect that correctly so we have to have test here also. Don't remove either one.
	assert := assert.New(t)

	runWithBadger(t, func(store *badger.DB, t *testing.T) {
		cache := NewCacheStore(store, time.Duration(1*time.Hour), true)
		sw := NewSpanWriter(store, cache, time.Duration(1*time.Hour), nil)
		rw := NewTraceReader(store, cache)

		ts := time.Now()
		q := &spanstore.TraceQueryParameters{
			StartTimeMax: ts,
			StartTimeMin: ts.Add(-1 * time.Hour),
		}
		links, err := rw.ScanDependencyIndex(q)
		assert.NoError(err)
		assert.Empty(links)

		traces := 80
		spans := 3
		s5c := 0
		for i := 0; i < traces; i++ {
			for j := 0; j < spans; j++ {
				s := model.Span{
					TraceID: model.TraceID{
						Low:  uint64(i),
						High: 1,
					},
					SpanID:        model.SpanID(j),
					OperationName: fmt.Sprintf("operation-a"),
					Process: &model.Process{
						ServiceName: fmt.Sprintf("service-%d", j),
					},
					StartTime: ts.Add(time.Minute * time.Duration(i)),
					Duration:  time.Duration(time.Millisecond * time.Duration(i+j)),
				}
				if j > 0 {
					s.References = []model.SpanRef{model.NewChildOfRef(s.TraceID, model.SpanID(j-1))}
				}
				if i%4 == 0 && j == 2 {
					s5c++
					s.Process.ServiceName = "service-5"
					if i%8 == 0 {
						s.OperationName = "operation-b"
					}
				}
				if i%8 == 0 {
					s.Tags = []model.KeyValue{
						model.KeyValue{
							Key:   "error",
							VBool: true,
							VType: model.ValueType_BOOL,
						},
					}
				}
				err := sw.WriteSpan(&s)
				assert.NoError(err)
			}
		}
		q.StartTimeMax = q.StartTimeMax.Add(time.Hour)
		links, err = rw.ScanDependencyIndex(q)
		assert.NoError(err)
		assert.NotEmpty(links)
		assert.Equal(spans, len(links))

		l := Link{
			From: "service-0",
			To:   "service-1",
		}

		count, found := links[l]
		assert.True(found)
		assert.Equal(61, int(count)) // Traces 0 -> 60 are in the calculation, but 61 -> 79 are not.

		q.ServiceName = "service-5"
		links, err = rw.ScanDependencyIndex(q)
		assert.NoError(err)
		assert.NotEmpty(links)

		count, found = links[l]
		assert.True(found)
		assert.Equal(16, int(count)) // Every fourth trace had the service only between traces 0 -> 60 (including 0 which is why it's 15+1)

		q.DurationMin = time.Duration(4) * time.Millisecond
		links, err = rw.ScanDependencyIndex(q)
		assert.NoError(err)
		assert.NotEmpty(links)

		count, found = links[l]
		assert.True(found)
		assert.Equal(15, int(count)) // Every fourth trace had the service only between traces 0 -> 60 (but 0 is excluded since its duration is not enough)

		q.OperationName = "operation-b"
		links, err = rw.ScanDependencyIndex(q)
		assert.NoError(err)
		assert.NotEmpty(links)

		count, found = links[l]
		assert.True(found)
		assert.Equal(7, int(count)) // Every eight trace had the service only between traces 0 -> 60 (excluding 0 because of duration min)

		// New scan, larger timeframe

		q = &spanstore.TraceQueryParameters{
			StartTimeMax: ts.Add(2 * time.Hour),
			StartTimeMin: ts.Add(-4 * time.Hour),
		}

		links, err = rw.ScanDependencyIndex(q)
		assert.NoError(err)
		assert.NotEmpty(links)
		assert.Equal(spans, len(links))

		count, found = links[l]
		assert.True(found)
		assert.Equal(80, int(count))

		q.Tags = map[string]string{
			"error": "true",
		}

		links, err = rw.ScanDependencyIndex(q)
		assert.NoError(err)
		assert.NotEmpty(links)
		assert.Equal(spans, len(links))

		count, found = links[l]
		assert.True(found)
		assert.Equal(80, int(count)) // No service defined, so tags filtering is ignored

		q.ServiceName = "service-5"
		links, err = rw.ScanDependencyIndex(q)
		assert.NoError(err)
		assert.NotEmpty(links)
		assert.Equal(spans, len(links))

		count, found = links[l]
		assert.True(found)
		assert.Equal(10, int(count)) // No service defined, so tags filtering is ignored
	})
}

func TestDecodeErrorReturns(t *testing.T) {
	garbage := []byte{0x08}

	_, err := decodeValue(garbage, protoEncoding)
	assert.Error(t, err)

	_, err = decodeValue(garbage, jsonEncoding)
	assert.Error(t, err)
}

func TestSortMergeIdsDuplicateDetection(t *testing.T) {
	// Different IndexSeeks return the same results
	ids := make([][][]byte, 2)
	ids[0] = make([][]byte, 1)
	ids[1] = make([][]byte, 1)
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, uint64(0))
	binary.Write(buf, binary.BigEndian, uint64(156697987635))
	b := buf.Bytes()
	ids[0][0] = b
	ids[1][0] = b

	query := &spanstore.TraceQueryParameters{
		NumTraces: 64,
	}

	traces := sortMergeIds(query, ids)
	assert.Equal(t, 1, len(traces))
}

func TestDuplicateTraceIDDetection(t *testing.T) {
	runWithBadger(t, func(store *badger.DB, t *testing.T) {
		testSpan := createDummySpan()
		cache := NewCacheStore(store, time.Duration(1*time.Hour), true)
		sw := NewSpanWriter(store, cache, time.Duration(1*time.Hour), nil)
		rw := NewTraceReader(store, cache)

		for i := 0; i < 8; i++ {
			testSpan.SpanID = model.SpanID(i)
			testSpan.StartTime = testSpan.StartTime.Add(time.Millisecond)
			err := sw.WriteSpan(&testSpan)
			assert.NoError(t, err)
		}

		traces, err := rw.FindTraceIDs(context.Background(), &spanstore.TraceQueryParameters{
			ServiceName:  "service",
			StartTimeMax: time.Now().Add(time.Hour),
			StartTimeMin: testSpan.StartTime.Add(-1 * time.Hour),
		})

		assert.NoError(t, err)
		assert.Equal(t, 1, len(traces))
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
	assert := assert.New(t)

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
	assert.Equal(16, len(merged))

	// Check order
	assert.Equal(uint32(15), binary.BigEndian.Uint32(merged[15]))

	// Test simple non-equality different size

	merged = mergeJoinIds(left[1:2], right[13:])
	assert.Empty(merged)

	// Different size, some equalities

	merged = mergeJoinIds(left[0:3], right[1:7])
	assert.Equal(2, len(merged))
	assert.Equal(uint32(2), binary.BigEndian.Uint32(merged[1]))
}
