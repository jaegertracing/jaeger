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
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
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
	depIndexKey           byte = 0x85
	jsonEncoding          byte = 0x01 // Last 4 bits of the meta byte are for encoding type
	protoEncoding         byte = 0x02 // Last 4 bits of the meta byte are for encoding type
	defaultEncoding       byte = protoEncoding
)

// SpanWriter for writing spans to badger
type SpanWriter struct {
	store        *badger.DB
	ttl          time.Duration
	cache        *CacheStore
	closer       io.Closer
	encodingType byte
}

// NewSpanWriter returns a SpawnWriter with cache
func NewSpanWriter(db *badger.DB, c *CacheStore, ttl time.Duration, storageCloser io.Closer) *SpanWriter {
	return &SpanWriter{
		store:        db,
		ttl:          ttl,
		cache:        c,
		closer:       storageCloser,
		encodingType: defaultEncoding, // TODO Make configurable
	}
}

// WriteSpan writes the encoded span as well as creates indexes with defined TTL
func (w *SpanWriter) WriteSpan(span *model.Span) error {

	// Avoid doing as much as possible inside the transaction boundary, create entries here
	entriesToStore := make([]*badger.Entry, 0, len(span.Tags)+4+len(span.Process.Tags)+len(span.Logs)*4)

	trace, err := w.createTraceEntry(span)
	if err != nil {
		return err
	}

	entriesToStore = append(entriesToStore, trace)
	entriesToStore = append(entriesToStore, w.createBadgerEntry(createIndexKey(serviceNameIndexKey, []byte(span.Process.ServiceName), span.StartTime, span.TraceID), nil))
	entriesToStore = append(entriesToStore, w.createBadgerEntry(createIndexKey(operationNameIndexKey, []byte(span.Process.ServiceName+span.OperationName), span.StartTime, span.TraceID), nil))

	// It doesn't matter if we overwrite Duration index keys, everything is read at Trace level in any case
	durationValue := make([]byte, 8)
	binary.BigEndian.PutUint64(durationValue, uint64(model.DurationAsMicroseconds(span.Duration)))
	entriesToStore = append(entriesToStore, w.createBadgerEntry(createIndexKey(durationIndexKey, durationValue, span.StartTime, span.TraceID), nil))

	for _, kv := range span.Tags {
		// Convert everything to string since queries are done that way also
		// KEY: it<serviceName><tagsKey><traceId> VALUE: <tagsValue>
		entriesToStore = append(entriesToStore, w.createBadgerEntry(createIndexKey(tagIndexKey, []byte(span.Process.ServiceName+kv.Key+kv.AsString()), span.StartTime, span.TraceID), nil))
	}

	for _, kv := range span.Process.Tags {
		entriesToStore = append(entriesToStore, w.createBadgerEntry(createIndexKey(tagIndexKey, []byte(span.Process.ServiceName+kv.Key+kv.AsString()), span.StartTime, span.TraceID), nil))
	}

	for _, log := range span.Logs {
		for _, kv := range log.Fields {
			entriesToStore = append(entriesToStore, w.createBadgerEntry(createIndexKey(tagIndexKey, []byte(span.Process.ServiceName+kv.Key+kv.AsString()), span.StartTime, span.TraceID), nil))
		}
	}

	// For dependency processing
	entriesToStore = append(entriesToStore, w.createBadgerEntry(createDependencyIndexKey(span), nil))

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
	w.cache.Update(span.Process.ServiceName, span.OperationName)

	return err
}

func createDependencyIndexKey(span *model.Span) []byte {
	// I need (for sorting purposes and optimization of reads):
	// depIndex<traceId><startTime><spanId><serviceName><parentSpanId> (if parentSpanId exists)

	buf := new(bytes.Buffer)

	buf.WriteByte(depIndexKey)
	binary.Write(buf, binary.BigEndian, span.TraceID.High)
	binary.Write(buf, binary.BigEndian, span.TraceID.Low)
	binary.Write(buf, binary.BigEndian, model.TimeAsEpochMicroseconds(span.StartTime))
	binary.Write(buf, binary.BigEndian, span.SpanID)
	binary.Write(buf, binary.BigEndian, []byte(span.Process.ServiceName))
	binary.Write(buf, binary.BigEndian, span.ParentSpanID())

	return buf.Bytes()
}

func createIndexKey(indexPrefixKey byte, value []byte, startTime time.Time, traceID model.TraceID) []byte {
	// KEY: indexKey<indexValue><startTime><traceId> (traceId is last 16 bytes of the key)
	buf := new(bytes.Buffer)

	buf.WriteByte((indexPrefixKey & indexKeyRange) | spanKeyPrefix) // Enforce to prevent future accidental key overlapping
	buf.Write(value)
	binary.Write(buf, binary.BigEndian, model.TimeAsEpochMicroseconds(startTime))
	binary.Write(buf, binary.BigEndian, traceID.High)
	binary.Write(buf, binary.BigEndian, traceID.Low)
	return buf.Bytes()
}

func (w *SpanWriter) createBadgerEntry(key []byte, value []byte) *badger.Entry {
	return &badger.Entry{
		Key:       key,
		Value:     value,
		ExpiresAt: uint64(time.Now().Add(w.ttl).Unix()),
	}
}

func (w *SpanWriter) createTraceEntry(span *model.Span) (*badger.Entry, error) {
	pK, pV, err := createTraceKV(span, w.encodingType)
	if err != nil {
		return nil, err
	}

	e := w.createBadgerEntry(pK, pV)
	e.UserMeta = w.encodingType

	return e, nil
}

func createTraceKV(span *model.Span, encodingType byte) ([]byte, []byte, error) {
	// TODO Add Hash for Zipkin compatibility?

	// Note, KEY must include startTime for proper sorting order for span-ids
	// KEY: ti<trace-id><startTime><span-id> VALUE: All the details (json for now) METADATA: Encoding
	buf := new(bytes.Buffer)

	buf.WriteByte(spanKeyPrefix)
	binary.Write(buf, binary.BigEndian, span.TraceID.High)
	binary.Write(buf, binary.BigEndian, span.TraceID.Low)
	binary.Write(buf, binary.BigEndian, model.TimeAsEpochMicroseconds(span.StartTime))
	binary.Write(buf, binary.BigEndian, span.SpanID)

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

	return buf.Bytes(), bb, err
}

// Close Implements io.Closer and closes the underlying storage
func (w *SpanWriter) Close() error {
	return w.closer.Close()
}
