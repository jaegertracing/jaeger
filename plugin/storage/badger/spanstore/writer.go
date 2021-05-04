// Copyright (c) 2018 The Jaeger Authors.
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
	"encoding/binary"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/gogo/protobuf/proto"

	"github.com/jaegertracing/jaeger/model"
)

/*
	This store should be easily modified to use any sorted KV-store, which allows set/get/iterators.
	That includes RocksDB also (this key structure should work as-is with RocksDB)

	Keys are written in BigEndian order to allow lexicographic sorting of keys
*/

const (
	spanKeyPrefix         byte = 0x80 // All span keys should have first bit set to 1
	indexKeyRange         byte = 0x0F // Secondary indexes use last 4 bits
	serviceNameIndexKey   byte = 0x81
	operationNameIndexKey byte = 0x82
	tagIndexKey           byte = 0x83
	durationIndexKey      byte = 0x84
	jsonEncoding          byte = 0x01 // Last 4 bits of the meta byte are for encoding type
	protoEncoding         byte = 0x02 // Last 4 bits of the meta byte are for encoding type
	defaultEncoding       byte = protoEncoding
)

// SpanWriter for writing spans to badger
type SpanWriter struct {
	store        *badger.DB
	ttl          time.Duration
	cache        *CacheStore
	encodingType byte
}

// NewSpanWriter returns a SpawnWriter with cache
func NewSpanWriter(db *badger.DB, c *CacheStore, ttl time.Duration) *SpanWriter {
	return &SpanWriter{
		store:        db,
		ttl:          ttl,
		cache:        c,
		encodingType: defaultEncoding, // TODO Make configurable
	}
}

// WriteSpan writes the encoded span as well as creates indexes with defined TTL
func (w *SpanWriter) WriteSpan(ctx context.Context, span *model.Span) error {
	expireTime := uint64(time.Now().Add(w.ttl).Unix())
	startTime := model.TimeAsEpochMicroseconds(span.StartTime)

	// Avoid doing as much as possible inside the transaction boundary, create entries here
	entriesToStore := make([]*badger.Entry, 0, len(span.Tags)+4+len(span.Process.Tags)+len(span.Logs)*4)

	trace, err := w.createTraceEntry(span, startTime, expireTime)
	if err != nil {
		return err
	}

	entriesToStore = append(entriesToStore, trace)
	entriesToStore = append(entriesToStore, w.createBadgerEntry(createIndexKey(serviceNameIndexKey, []byte(span.Process.ServiceName), startTime, span.TraceID), nil, expireTime))
	entriesToStore = append(entriesToStore, w.createBadgerEntry(createIndexKey(operationNameIndexKey, []byte(span.Process.ServiceName+span.OperationName), startTime, span.TraceID), nil, expireTime))

	// It doesn't matter if we overwrite Duration index keys, everything is read at Trace level in any case
	durationValue := make([]byte, 8)
	binary.BigEndian.PutUint64(durationValue, uint64(model.DurationAsMicroseconds(span.Duration)))
	entriesToStore = append(entriesToStore, w.createBadgerEntry(createIndexKey(durationIndexKey, durationValue, startTime, span.TraceID), nil, expireTime))

	for _, kv := range span.Tags {
		// Convert everything to string since queries are done that way also
		// KEY: it<serviceName><tagsKey><traceId> VALUE: <tagsValue>
		entriesToStore = append(entriesToStore, w.createBadgerEntry(createIndexKey(tagIndexKey, []byte(span.Process.ServiceName+kv.Key+kv.AsString()), startTime, span.TraceID), nil, expireTime))
	}

	for _, kv := range span.Process.Tags {
		entriesToStore = append(entriesToStore, w.createBadgerEntry(createIndexKey(tagIndexKey, []byte(span.Process.ServiceName+kv.Key+kv.AsString()), startTime, span.TraceID), nil, expireTime))
	}

	for _, log := range span.Logs {
		for _, kv := range log.Fields {
			entriesToStore = append(entriesToStore, w.createBadgerEntry(createIndexKey(tagIndexKey, []byte(span.Process.ServiceName+kv.Key+kv.AsString()), startTime, span.TraceID), nil, expireTime))
		}
	}

	err = w.store.Update(func(txn *badger.Txn) error {
		// Write the entries
		for i := range entriesToStore {
			err = txn.SetEntry(entriesToStore[i])
			if err != nil {
				// Most likely primary key conflict, but let the caller check this
				return err
			}
		}

		// TODO Alternative option is to use simpler keys with the merge value interface.
		// Requires at least this to be solved: https://github.com/dgraph-io/badger/issues/373

		return nil
	})

	// Do cache refresh here to release the transaction earlier
	w.cache.Update(span.Process.ServiceName, span.OperationName, expireTime)

	return err
}

func createIndexKey(indexPrefixKey byte, value []byte, startTime uint64, traceID model.TraceID) []byte {
	// KEY: indexKey<indexValue><startTime><traceId> (traceId is last 16 bytes of the key)
	key := make([]byte, 1+len(value)+8+sizeOfTraceID)
	key[0] = (indexPrefixKey & indexKeyRange) | spanKeyPrefix
	pos := len(value) + 1
	copy(key[1:pos], value)
	binary.BigEndian.PutUint64(key[pos:], startTime)
	pos += 8 // sizeOfTraceID / 2
	binary.BigEndian.PutUint64(key[pos:], traceID.High)
	pos += 8 // sizeOfTraceID / 2
	binary.BigEndian.PutUint64(key[pos:], traceID.Low)
	return key
}

func (w *SpanWriter) createBadgerEntry(key []byte, value []byte, expireTime uint64) *badger.Entry {
	return &badger.Entry{
		Key:       key,
		Value:     value,
		ExpiresAt: expireTime,
	}
}

func (w *SpanWriter) createTraceEntry(span *model.Span, startTime, expireTime uint64) (*badger.Entry, error) {
	pK, pV, err := createTraceKV(span, w.encodingType, startTime)
	if err != nil {
		return nil, err
	}

	e := w.createBadgerEntry(pK, pV, expireTime)
	e.UserMeta = w.encodingType

	return e, nil
}

func createTraceKV(span *model.Span, encodingType byte, startTime uint64) ([]byte, []byte, error) {
	// TODO Add Hash for Zipkin compatibility?

	// Note, KEY must include startTime for proper sorting order for span-ids
	// KEY: ti<trace-id><startTime><span-id> VALUE: All the details (json for now) METADATA: Encoding

	key := make([]byte, 1+sizeOfTraceID+8+8)
	key[0] = spanKeyPrefix
	pos := 1
	binary.BigEndian.PutUint64(key[pos:], span.TraceID.High)
	pos += 8
	binary.BigEndian.PutUint64(key[pos:], span.TraceID.Low)
	pos += 8
	binary.BigEndian.PutUint64(key[pos:], startTime)
	pos += 8
	binary.BigEndian.PutUint64(key[pos:], uint64(span.SpanID))

	var bb []byte
	var err error

	switch encodingType {
	case protoEncoding:
		bb, err = proto.Marshal(span)
	case jsonEncoding:
		bb, err = json.Marshal(span)
	default:
		return nil, nil, fmt.Errorf("unknown encoding type: %#02x", encodingType)
	}

	return key, bb, err
}
