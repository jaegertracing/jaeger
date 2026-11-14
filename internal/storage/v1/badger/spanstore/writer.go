// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/gogo/protobuf/proto"

	"github.com/jaegertracing/jaeger-idl/model/v1"
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
	//nolint:gosec // G115
	expireTime := uint64(time.Now().Add(w.ttl).Unix())
	startTime := model.TimeAsEpochMicroseconds(span.StartTime)

	// Avoid doing as much as possible inside the transaction boundary, create entries here
	entriesToStore := make([]*badger.Entry, 0, len(span.Tags)+4+len(span.Process.Tags)+len(span.Logs)*4)

	trace, err := w.createTraceEntry(span, startTime, expireTime)
	if err != nil {
		return err
	}

	entriesToStore = append(entriesToStore, trace)
	serviceKey := createIndexKey(serviceNameIndexKey, []byte(span.Process.ServiceName), startTime, span.TraceID)
	operationKey := createIndexKey(operationNameIndexKey, []byte(span.Process.ServiceName+span.OperationName), startTime, span.TraceID)
	kind := spanKind(span)

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

	var kinds operationKinds
	var operationExpiry, serviceExpiry uint64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err = w.store.Update(func(txn *badger.Txn) error {
			var err error
			kinds, operationExpiry, err = mergeOperationIndex(txn, operationKey, kind, expireTime)
			if err != nil {
				return err
			}
			serviceExpiry, err = mergeServiceExpiry(txn, serviceKey, operationExpiry)
			if err != nil {
				return err
			}
			if err := txn.SetEntry(w.createBadgerEntry(operationKey, kinds.encode(), operationExpiry)); err != nil {
				return err
			}
			if err := txn.SetEntry(w.createBadgerEntry(serviceKey, nil, serviceExpiry)); err != nil {
				return err
			}
			// Write the entries
			for i := range entriesToStore {
				err = txn.SetEntry(entriesToStore[i])
				if err != nil {
					return err
				}
			}

			return nil
		})
		if !errors.Is(err, badger.ErrConflict) {
			break
		}
	}
	if err != nil {
		return err
	}

	// Do cache refresh here to release the transaction earlier
	w.cache.updateOperations(span.Process.ServiceName, span.OperationName, kinds.kinds, serviceExpiry, operationExpiry)

	return nil
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

func (*SpanWriter) createBadgerEntry(key []byte, value []byte, expireTime uint64) *badger.Entry {
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

func createTraceKV(span *model.Span, encodingType byte, startTime uint64) (key []byte, bb []byte, err error) {
	// TODO Add Hash for Zipkin compatibility?

	// Note, KEY must include startTime for proper sorting order for span-ids
	// KEY: ti<trace-id><startTime><span-id> VALUE: All the details (json for now) METADATA: Encoding

	key = make([]byte, 1+sizeOfTraceID+8+8)
	key[0] = spanKeyPrefix
	pos := 1
	binary.BigEndian.PutUint64(key[pos:], span.TraceID.High)
	pos += 8
	binary.BigEndian.PutUint64(key[pos:], span.TraceID.Low)
	pos += 8
	binary.BigEndian.PutUint64(key[pos:], startTime)
	pos += 8
	binary.BigEndian.PutUint64(key[pos:], uint64(span.SpanID))

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
