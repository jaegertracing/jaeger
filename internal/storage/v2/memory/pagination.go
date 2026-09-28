// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package memory

import (
	"bytes"
	"cmp"
	"encoding/binary"
	"fmt"
	"math"
	"sort"

	"go.opentelemetry.io/collector/pdata/pcommon"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// key is a sort key that a cursor can carry: sortingKey or traceKey.
type key interface {
	encode() []byte
}

// cursor is where a page ended: the key of its last element and how many elements with that key
// the pages so far have returned. A span stored twice shares its whole key with its copy, so
// the count is what lets the next page resume at the second copy rather than skip it (RFC 0016
// §6) while PageSize stays the bound on every page. Encoded, it is the cursor inside a page token.
type cursor[K key] struct {
	key  K
	seen uint32
}

// sortingKey holds one encoded value per ordering term, in query precedence order. Each value's
// byte order matches its ascending sort order, so comparing and encoding a key depends on how
// many terms the query has and never on which fields they name.
type sortingKey [][]byte

// traceKey sorts traces by the latest start time among their matching spans descending, then
// trace ID ascending (RFC 0014 §3.3).
type traceKey struct {
	startTime pcommon.Timestamp
	traceID   pcommon.TraceID
}

const traceKeySize = 8 + 16

// makeSortingKey resolves each ordering term's expression against the span the way the filter
// evaluator resolves an operand, so any field or attribute the evaluator knows can order spans,
// and encodes each value so that byte order is sort order.
func makeSortingKey(ctx filterCtx, order []tracestore.SpanSortOrder) sortingKey {
	key := make(sortingKey, len(order))
	for i, expr := range order {
		key[i] = encodeSortValue(resolveOperand(expr.Expression, ctx))
	}
	return key
}

// encodeSortValue encodes the first resolved value so that bytes.Compare orders encodings as the
// values themselves are ordered. The kind is the leading byte, so values of different kinds keep
// the fixed relative order evalKind declares, with a span that lacks the value first. Integers get
// their sign bit flipped; doubles get their sign bit flipped when positive and every bit flipped
// when negative, which is the standard order-preserving encoding of IEEE 754 floats; strings are
// their own bytes; opaque values carry only their kind.
func encodeSortValue(values []evalValue) []byte {
	if len(values) == 0 {
		return []byte{byte(kindNone)}
	}
	v := values[0]
	tag := []byte{byte(v.kind)}
	switch v.kind {
	case kindBool:
		if v.boolean {
			return append(tag, 1)
		}
		return append(tag, 0)
	case kindInt:
		return binary.BigEndian.AppendUint64(tag, uint64(v.numInt)^(1<<63)) //nolint:gosec // G115: This bit cast preserves the signed order.
	case kindDouble:
		bits := math.Float64bits(v.num)
		if bits&(1<<63) != 0 {
			bits = ^bits
		} else {
			bits |= 1 << 63
		}
		return binary.BigEndian.AppendUint64(tag, bits)
	case kindString, kindUntyped:
		return append(tag, v.str...)
	default:
		return tag
	}
}

func compareSortingKeys(order []tracestore.SpanSortOrder) func(sortingKey, sortingKey) int {
	return func(a, b sortingKey) int {
		for i, term := range order {
			result := bytes.Compare(a[i], b[i])
			if term.Direction == tracestore.SortDescending {
				result = -result
			}
			if result != 0 {
				return result
			}
		}
		return 0
	}
}

func (k sortingKey) encode() []byte {
	var buf []byte
	for _, value := range k {
		buf = binary.AppendUvarint(buf, uint64(len(value)))
		buf = append(buf, value...)
	}
	return buf
}

func decodeSpanCursor(raw []byte, terms int) (cursor[sortingKey], error) {
	var key sortingKey
	for range terms {
		size, n := binary.Uvarint(raw)
		if n <= 0 {
			return cursor[sortingKey]{}, fmt.Errorf("%w: page token does not carry a span position", tracestore.ErrPaginationInvalid)
		}
		raw = raw[n:]
		if size > uint64(len(raw)) {
			return cursor[sortingKey]{}, fmt.Errorf("%w: page token does not carry a span position", tracestore.ErrPaginationInvalid)
		}
		key = append(key, raw[:size])
		raw = raw[size:]
	}
	if len(raw) != 4 {
		return cursor[sortingKey]{}, fmt.Errorf("%w: page token does not carry a span position", tracestore.ErrPaginationInvalid)
	}
	return cursor[sortingKey]{key: key, seen: binary.BigEndian.Uint32(raw)}, nil
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

func decodeTraceKey(raw []byte) traceKey {
	var k traceKey
	k.startTime = pcommon.Timestamp(binary.BigEndian.Uint64(raw))
	copy(k.traceID[:], raw[8:])
	return k
}

func decodeTraceCursor(raw []byte) (cursor[traceKey], error) {
	return decodeCursor(raw, traceKeySize, decodeTraceKey, "trace")
}

func (c cursor[K]) encode() []byte {
	return binary.BigEndian.AppendUint32(c.key.encode(), c.seen)
}

// decodeCursor reads a cursor whose key takes keySize bytes; what names the kind of key for the
// error a token of another shape gets.
func decodeCursor[K key](raw []byte, keySize int, decodeKey func([]byte) K, what string) (cursor[K], error) {
	if len(raw) != keySize+4 {
		return cursor[K]{}, fmt.Errorf("%w: page token does not carry a %s position", tracestore.ErrPaginationInvalid, what)
	}
	return cursor[K]{key: decodeKey(raw[:keySize]), seen: binary.BigEndian.Uint32(raw[keySize:])}, nil
}

// cursorOf decodes the cursor carried by the page token, or returns nil for an empty token. The
// token must have been returned for the same query.
func cursorOf[K key](token tracestore.PageToken, fingerprint []byte, decode func([]byte) (cursor[K], error)) (*cursor[K], error) {
	if token == "" {
		return nil, nil
	}
	raw, err := token.Cursor(fingerprint)
	if err != nil {
		return nil, err
	}
	c, err := decode(raw)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// page returns the elements that follow the cursor after (all elements when after is nil), at
// most size of them when size is positive. If elements remain beyond the page, it also returns
// the next page's cursor: the key of the page's last element, and how many elements with that
// key have been returned so far.
func page[T any, K key](sorted []T, keyOf func(T) K, compare func(K, K) int, after *cursor[K], size uint32) ([]T, *cursor[K]) {
	start := 0
	if after != nil {
		// Resume at the first element with the cursor's key and skip the copies of it already
		// returned. If fewer copies remain than were returned, the skip stops at the next key.
		start = sort.Search(len(sorted), func(i int) bool { return compare(keyOf(sorted[i]), after.key) >= 0 })
		for skipped := uint32(0); skipped < after.seen && start < len(sorted) && compare(keyOf(sorted[start]), after.key) == 0; skipped++ {
			start++
		}
	}
	rest := sorted[start:]
	if size == 0 || uint64(len(rest)) <= uint64(size) {
		return rest, nil
	}
	next := cursor[K]{key: keyOf(rest[size-1])}
	for i := size; i > 0 && compare(keyOf(rest[i-1]), next.key) == 0; i-- {
		next.seen++
	}
	if after != nil && compare(next.key, after.key) == 0 {
		next.seen += after.seen
	}
	return rest[:size], &next
}
