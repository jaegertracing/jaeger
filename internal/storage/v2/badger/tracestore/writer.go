// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"encoding/binary"
	"time"

	"github.com/dgraph-io/badger/v4"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	conventions "github.com/jaegertracing/jaeger/internal/telemetry/otelsemconv"
)

/*
Key schema for native v2 Badger storage:

Span keys: spanKeyPrefix<trace-id><startTime><span-id>
  - Value: protobuf-encoded ptrace.Traces containing the span with its resource/scope

Index keys (empty values, using \x00 separator between variable-length fields):
  - Service: serviceNameIndexKey<serviceName><startTime><traceId>
  - Operation: operationNameIndexKey<serviceName>\x00<operationName><startTime><traceId>
  - Tag: tagIndexKey<serviceName>\x00<tagKey>\x00<tagValue><startTime><traceId>
  - Duration: durationIndexKey<duration><startTime><traceId>
*/

const (
	spanKeyPrefix         byte = 0x80
	indexKeyRange         byte = 0x0F
	serviceNameIndexKey   byte = 0x81
	operationNameIndexKey byte = 0x82
	tagIndexKey           byte = 0x83
	durationIndexKey      byte = 0x84

	sizeOfTraceID = 16
	sizeOfSpanID  = 8

	// separator is used between variable-length fields in index keys
	// to prevent key collisions (e.g., "foo"+"bar" vs "foob"+"ar")
	separator = "\x00"
)

var _ tracestore.Writer = (*TraceWriter)(nil)

// TraceWriter writes traces to badger storage in native OTLP format.
type TraceWriter struct {
	store *badger.DB
	ttl   time.Duration
}

// NewTraceWriter creates a new TraceWriter.
func NewTraceWriter(db *badger.DB, ttl time.Duration) *TraceWriter {
	return &TraceWriter{
		store: db,
		ttl:   ttl,
	}
}

// WriteTraces writes a batch of traces to storage in OTLP format.
func (w *TraceWriter) WriteTraces(_ context.Context, td ptrace.Traces) error {
	//nolint:gosec // G115
	expireTime := uint64(time.Now().Add(w.ttl).Unix())

	return w.store.Update(func(txn *badger.Txn) error {
		for _, rs := range td.ResourceSpans().All() {
			serviceName := getServiceName(rs.Resource())
			for _, ss := range rs.ScopeSpans().All() {
				for _, span := range ss.Spans().All() {
					if err := w.writeSpan(txn, rs, ss, span, serviceName, expireTime); err != nil {
						return err
					}
				}
			}
		}
		return nil
	})
}

func (*TraceWriter) writeSpan(
	txn *badger.Txn,
	rs ptrace.ResourceSpans,
	ss ptrace.ScopeSpans,
	span ptrace.Span,
	serviceName string,
	expireTime uint64,
) error {
	traceID := span.TraceID()
	spanID := span.SpanID()
	startTime := uint64(span.StartTimestamp())

	// Serialize span with its resource and scope context
	spanBytes, err := marshalSpan(rs, ss, span)
	if err != nil {
		return err
	}

	// Write primary span entry
	spanKey := createSpanKey(traceID, startTime, spanID)
	if err := setEntry(txn, spanKey, spanBytes, expireTime); err != nil {
		return err
	}

	// Write service index
	if serviceName != "" {
		serviceKey := createIndexKey(serviceNameIndexKey, []byte(serviceName), startTime, traceID)
		if err := setEntry(txn, serviceKey, nil, expireTime); err != nil {
			return err
		}

		// Write operation index
		operationName := span.Name()
		if operationName != "" {
			opKey := createIndexKey(operationNameIndexKey, []byte(serviceName+separator+operationName), startTime, traceID)
			if err := setEntry(txn, opKey, nil, expireTime); err != nil {
				return err
			}
		}

		// Write tag indexes from span attributes
		for key, val := range span.Attributes().All() {
			tagKey := createIndexKey(tagIndexKey, []byte(serviceName+separator+key+separator+val.AsString()), startTime, traceID)
			if err := setEntry(txn, tagKey, nil, expireTime); err != nil {
				return err
			}
		}

		// Write tag indexes from resource attributes (excluding service name)
		for key, val := range rs.Resource().Attributes().All() {
			if key == conventions.ServiceNameKey {
				continue
			}
			tagKey := createIndexKey(tagIndexKey, []byte(serviceName+separator+key+separator+val.AsString()), startTime, traceID)
			if err := setEntry(txn, tagKey, nil, expireTime); err != nil {
				return err
			}
		}

		// Write tag indexes from events
		for _, event := range span.Events().All() {
			for key, val := range event.Attributes().All() {
				tagKey := createIndexKey(tagIndexKey, []byte(serviceName+separator+key+separator+val.AsString()), startTime, traceID)
				if err := setEntry(txn, tagKey, nil, expireTime); err != nil {
					return err
				}
			}
		}
	}

	// Write duration index (always written, independent of serviceName,
	// to support duration-based queries without service filter)
	startTS := span.StartTimestamp()
	endTS := span.EndTimestamp()
	var duration uint64
	if endTS >= startTS {
		duration = uint64(endTS - startTS)
	} else {
		duration = 0
	}
	durationBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(durationBytes, duration)
	durationKey := createIndexKey(durationIndexKey, durationBytes, startTime, traceID)
	return setEntry(txn, durationKey, nil, expireTime)
}

func setEntry(txn *badger.Txn, key, value []byte, expireTime uint64) error {
	return txn.SetEntry(&badger.Entry{
		Key:       key,
		Value:     value,
		ExpiresAt: expireTime,
	})
}

func createSpanKey(traceID pcommon.TraceID, startTime uint64, spanID pcommon.SpanID) []byte {
	key := make([]byte, 1+sizeOfTraceID+8+sizeOfSpanID)
	key[0] = spanKeyPrefix
	pos := 1
	copy(key[pos:], traceID[:])
	pos += sizeOfTraceID
	binary.BigEndian.PutUint64(key[pos:], startTime)
	pos += 8
	copy(key[pos:], spanID[:])
	return key
}

func createIndexKey(indexPrefix byte, value []byte, startTime uint64, traceID pcommon.TraceID) []byte {
	key := make([]byte, 1+len(value)+8+sizeOfTraceID)
	key[0] = (indexPrefix & indexKeyRange) | spanKeyPrefix
	pos := 1
	copy(key[pos:], value)
	pos += len(value)
	binary.BigEndian.PutUint64(key[pos:], startTime)
	pos += 8
	copy(key[pos:], traceID[:])
	return key
}

func marshalSpan(rs ptrace.ResourceSpans, ss ptrace.ScopeSpans, span ptrace.Span) ([]byte, error) {
	td := ptrace.NewTraces()
	newRS := td.ResourceSpans().AppendEmpty()
	rs.Resource().CopyTo(newRS.Resource())
	newRS.SetSchemaUrl(rs.SchemaUrl())

	newSS := newRS.ScopeSpans().AppendEmpty()
	ss.Scope().CopyTo(newSS.Scope())
	newSS.SetSchemaUrl(ss.SchemaUrl())

	span.CopyTo(newSS.Spans().AppendEmpty())

	marshaler := &ptrace.ProtoMarshaler{}
	return marshaler.MarshalTraces(td)
}

func getServiceName(resource pcommon.Resource) string {
	if val, ok := resource.Attributes().Get(conventions.ServiceNameKey); ok {
		return val.Str()
	}
	return ""
}
