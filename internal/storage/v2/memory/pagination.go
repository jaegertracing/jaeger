// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package memory

import (
	"bytes"
	"cmp"
	"encoding/binary"
	"fmt"
	"sort"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// spanKey sorts spans by start time descending, then trace ID and span ID ascending (RFC 0016
// §6). Encoded, it is the cursor inside a page token.
type spanKey struct {
	startTime pcommon.Timestamp
	traceID   pcommon.TraceID
	spanID    pcommon.SpanID
}

const spanKeySize = 8 + 16 + 8

// traceKey sorts traces by the latest start time among their matching spans descending, then
// trace ID ascending (RFC 0014 §3.3). Encoded, it is the cursor inside a page token.
type traceKey struct {
	startTime pcommon.Timestamp
	traceID   pcommon.TraceID
}

const traceKeySize = 8 + 16

func spanKeyOf(span ptrace.Span) spanKey {
	return spanKey{startTime: span.StartTimestamp(), traceID: span.TraceID(), spanID: span.SpanID()}
}

func compareSpanKeys(a, b spanKey) int {
	if c := cmp.Compare(b.startTime, a.startTime); c != 0 {
		return c
	}
	if c := bytes.Compare(a.traceID[:], b.traceID[:]); c != 0 {
		return c
	}
	return bytes.Compare(a.spanID[:], b.spanID[:])
}

func (k spanKey) encode() []byte {
	buf := make([]byte, 0, spanKeySize)
	buf = binary.BigEndian.AppendUint64(buf, uint64(k.startTime))
	buf = append(buf, k.traceID[:]...)
	return append(buf, k.spanID[:]...)
}

func decodeSpanKey(raw []byte) (spanKey, error) {
	if len(raw) != spanKeySize {
		return spanKey{}, fmt.Errorf("%w: page token does not carry a span position", tracestore.ErrPaginationInvalid)
	}
	var k spanKey
	k.startTime = pcommon.Timestamp(binary.BigEndian.Uint64(raw))
	copy(k.traceID[:], raw[8:24])
	copy(k.spanID[:], raw[24:])
	return k, nil
}

func compareTraceKeys(a, b traceKey) int {
	if c := cmp.Compare(b.startTime, a.startTime); c != 0 {
		return c
	}
	return bytes.Compare(a.traceID[:], b.traceID[:])
}

func (k traceKey) encode() []byte {
	buf := make([]byte, 0, traceKeySize)
	buf = binary.BigEndian.AppendUint64(buf, uint64(k.startTime))
	return append(buf, k.traceID[:]...)
}

func decodeTraceKey(raw []byte) (traceKey, error) {
	if len(raw) != traceKeySize {
		return traceKey{}, fmt.Errorf("%w: page token does not carry a trace position", tracestore.ErrPaginationInvalid)
	}
	var k traceKey
	k.startTime = pcommon.Timestamp(binary.BigEndian.Uint64(raw))
	copy(k.traceID[:], raw[8:])
	return k, nil
}

// cursorOf decodes the key carried by the page token, or returns nil for an empty token. The
// token must have been returned for the same query.
func cursorOf[K any](token tracestore.PageToken, fingerprint []byte, decode func([]byte) (K, error)) (*K, error) {
	if token == "" {
		return nil, nil
	}
	raw, err := token.Cursor(fingerprint)
	if err != nil {
		return nil, err
	}
	key, err := decode(raw)
	if err != nil {
		return nil, err
	}
	return &key, nil
}

// page returns the elements that follow the key after (all elements when after is nil), size of
// them when size is positive. If elements remain beyond the page, it also returns the key of the
// page's last element, the next page's cursor. Elements that share that key stay on the page,
// since the cursor cannot resume between them: a span stored twice is returned twice rather than
// skipped (RFC 0016 §6), so the page grows past size by the number of duplicates.
func page[T any, K any](sorted []T, keyOf func(T) K, compare func(K, K) int, after *K, size int) ([]T, *K) {
	start := 0
	if after != nil {
		start = sort.Search(len(sorted), func(i int) bool { return compare(keyOf(sorted[i]), *after) > 0 })
	}
	rest := sorted[start:]
	if size <= 0 || len(rest) <= size {
		return rest, nil
	}
	last := keyOf(rest[size-1])
	end := size
	for end < len(rest) && compare(keyOf(rest[end]), last) == 0 {
		end++
	}
	if end == len(rest) {
		return rest, nil
	}
	return rest[:end], &last
}
