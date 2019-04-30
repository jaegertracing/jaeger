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
	"context"
	"testing"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/model"
)

func TestEncodingTypes(t *testing.T) {

	tid := time.Now()

	dummyKv := []model.KeyValue{
		model.KeyValue{
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
			model.Log{
				Timestamp: tid,
				Fields:    dummyKv,
			},
		},
	}

	// JSON encoding
	runWithBadger(t, func(store *badger.DB, t *testing.T) {
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
		cache := NewCacheStore(store, time.Duration(1*time.Hour), true)
		sw := NewSpanWriter(store, cache, time.Duration(1*time.Hour), nil)
		// rw := NewTraceReader(store, cache)

		sw.encodingType = 0x04
		err := sw.WriteSpan(&testSpan)
		assert.EqualError(t, err, "unknown encoding type: 0x04")
	})

	// Unknown encoding reader
	runWithBadger(t, func(store *badger.DB, t *testing.T) {
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

func TestDecodeErrorReturns(t *testing.T) {
	garbage := []byte{0x08}

	_, err := decodeValue(garbage, protoEncoding)
	assert.Error(t, err)

	_, err = decodeValue(garbage, jsonEncoding)
	assert.Error(t, err)
}
