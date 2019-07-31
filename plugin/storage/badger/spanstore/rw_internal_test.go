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

func TestScanIndexKey(t *testing.T) {
	// This functionality is tested through dependencystore's storage_test.go, but the codecov
	// can't detect that correctly so we have to have test here also. Don't remove either one.
	assert := assert.New(t)

	runWithBadger(t, func(store *badger.DB, t *testing.T) {
		cache := NewCacheStore(store, time.Duration(1*time.Hour), true)
		sw := NewSpanWriter(store, cache, time.Duration(1*time.Hour), nil)
		rw := NewTraceReader(store, cache)

		ts := time.Now()
		links, err := rw.ScanDependencyIndex(ts.Add(-1*time.Hour), ts)
		assert.NoError(err)
		assert.Empty(links)

		traces := 80
		spans := 3
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
					Duration:  time.Duration(i + j),
				}
				if j > 0 {
					s.References = []model.SpanRef{model.NewChildOfRef(s.TraceID, model.SpanID(j-1))}
				}
				err := sw.WriteSpan(&s)
				assert.NoError(err)
			}
		}
		links, err = rw.ScanDependencyIndex(ts.Add(-1*time.Hour), ts.Add(time.Hour))
		assert.NoError(err)
		assert.NotEmpty(links)
		assert.Equal(spans-1, len(links)) // First span does not create a link

		l := Link{
			From: "service-0",
			To:   "service-1",
		}

		count, found := links[l]
		assert.True(found)
		assert.Equal(61, int(count)) // Traces 0 -> 60 are in the calculation, but 61 -> 79 are not.
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
