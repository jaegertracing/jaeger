// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeIndexFields_CollidingConcatenationsDiffer(t *testing.T) {
	// "A"+"Bc" and "AB"+"c" both concatenate to "ABc" with plain string
	// concatenation; encodeIndexFields must keep them distinct.
	a := encodeIndexFields("A", "Bc")
	b := encodeIndexFields("AB", "c")
	assert.NotEqual(t, a, b)

	// "checkout"+"id"+"123" and "checkout"+"id1"+"23" both concatenate to
	// "checkoutid123".
	c := encodeIndexFields("checkout", "id", "123")
	d := encodeIndexFields("checkout", "id1", "23")
	assert.NotEqual(t, c, d)
}

func TestEncodeIndexFields_SameFieldsProduceSameBytes(t *testing.T) {
	first := encodeIndexFields("A", "Bc")
	second := encodeIndexFields("A", "Bc")
	assert.Equal(t, first, second)
}

func TestEncodeIndexFields_DecodeRoundTrip(t *testing.T) {
	fields := []string{"checkout", "id", "123"}
	encoded := encodeIndexFields(fields...)

	offset := 0
	for _, want := range fields {
		var got []byte
		var ok bool
		got, offset, ok = decodeIndexField(encoded, offset)
		require.True(t, ok)
		assert.Equal(t, want, string(got))
	}
	assert.Len(t, encoded, offset, "decoding should consume exactly the encoded bytes")
}

func TestDecodeIndexField_RejectsOldUnprefixedFormat(t *testing.T) {
	// Old-format index entries concatenated fields directly, with no length
	// prefix, e.g. raw "service1". The first two bytes ('s','e' = 0x7365)
	// decode as a bogus length far past the buffer; decodeIndexField must
	// report failure instead of slicing out of range.
	old := []byte("service1")

	_, _, ok := decodeIndexField(old, 0)
	assert.False(t, ok)
}

func TestDecodeIndexField_RejectsTruncatedInput(t *testing.T) {
	// Fewer than 2 bytes remain to hold the length prefix at all.
	_, _, ok := decodeIndexField([]byte{0x00}, 0)
	assert.False(t, ok)

	// A valid-looking length prefix whose declared length runs past the end
	// of the buffer (declares 10 bytes, only 3 are present).
	truncated := []byte{0x00, 0x0A, 'a', 'b', 'c'}
	_, _, ok = decodeIndexField(truncated, 0)
	assert.False(t, ok)

	// Offset itself is past the end of the buffer.
	_, _, ok = decodeIndexField([]byte{0x00, 0x01, 'a'}, 10)
	assert.False(t, ok)
}

func TestEncodeIndexFields_SingleFieldIsPrefixOfMultiField(t *testing.T) {
	// preloadOperations relies on encodeIndexFields(service) being an exact byte
	// prefix of encodeIndexFields(service, operation), for any operation, so it
	// can use the former as a Badger iterator seek prefix.
	serviceOnly := encodeIndexFields("checkout")
	serviceAndOp := encodeIndexFields("checkout", "place-order")
	assert.True(t, strings.HasPrefix(string(serviceAndOp), string(serviceOnly)))
}

func TestEncodeIndexFields_EmptyFields(t *testing.T) {
	encoded := encodeIndexFields("", "x", "")
	offset := 0
	var got []byte
	var ok bool
	got, offset, ok = decodeIndexField(encoded, offset)
	require.True(t, ok)
	assert.Empty(t, got)
	got, offset, ok = decodeIndexField(encoded, offset)
	require.True(t, ok)
	assert.Equal(t, "x", string(got))
	got, offset, ok = decodeIndexField(encoded, offset)
	require.True(t, ok)
	assert.Empty(t, got)
	assert.Len(t, encoded, offset)
}

func TestEncodeIndexFields_TruncatesOverlongFieldsWithoutDrift(t *testing.T) {
	long := strings.Repeat("a", maxIndexFieldLen+100)
	encoded := encodeIndexFields(long, "next")

	field, offset, ok := decodeIndexField(encoded, 0)
	require.True(t, ok)
	require.Len(t, field, maxIndexFieldLen, "field must be truncated to the max length")

	// The encoded length must match the truncated bytes exactly, so decoding the
	// next field still lands in the right place.
	next, _, ok := decodeIndexField(encoded, offset)
	require.True(t, ok)
	assert.Equal(t, "next", string(next))
}
