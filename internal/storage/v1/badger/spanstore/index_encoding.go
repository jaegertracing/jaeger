// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import "encoding/binary"

// maxIndexFieldLen is the largest field length that fits in the 2-byte
// big-endian length prefix used by encodeIndexFields.
const maxIndexFieldLen = 65535

// encodeIndexFields concatenates fields into a single byte slice, prefixing each
// field with its own 2-byte big-endian length so that field boundaries are
// recoverable. Without explicit boundaries, different field combinations can
// concatenate to identical bytes -- e.g. ("A","Bc") and ("AB","c") both produce
// "ABc" -- making them indistinguishable once written into an index key, so a
// query for one pair can silently match spans belonging to the other.
//
// A field longer than maxIndexFieldLen is truncated before encoding, and the
// encoded length always matches the truncated bytes. Truncation can only cause
// two very long fields to collide with each other; it cannot corrupt or misalign
// any other index key, since every field's length is explicit in the encoding.
func encodeIndexFields(fields ...string) []byte {
	total := 0
	for _, f := range fields {
		total += 2 + truncatedLen(f)
	}

	out := make([]byte, 0, total)
	for _, f := range fields {
		b := []byte(f)
		if len(b) > maxIndexFieldLen {
			b = b[:maxIndexFieldLen]
		}
		var lenBuf [2]byte
		//nolint:gosec // G115: len(b) <= maxIndexFieldLen (65535) by the truncation above
		binary.BigEndian.PutUint16(lenBuf[:], uint16(len(b)))
		out = append(out, lenBuf[:]...)
		out = append(out, b...)
	}
	return out
}

func truncatedLen(f string) int {
	if len(f) > maxIndexFieldLen {
		return maxIndexFieldLen
	}
	return len(f)
}

// decodeIndexField reads a single length-prefixed field encoded by
// encodeIndexFields, starting at offset in b. It returns the field's bytes and
// the offset immediately following the field, so callers can decode a sequence
// of fields by chaining calls.
//
// ok is false if b does not contain a valid length-prefixed field at offset --
// too few bytes remain to hold the 2-byte length, or the declared length runs
// past the end of b. This is not just defensive: it is the only thing standing
// between a stray badger entry in the old, unprefixed key format (e.g. left over
// from before this encoding existed) and an out-of-range panic, since the first
// two bytes of arbitrary old-format data are read as a length and used as a
// slice bound. Callers must treat ok==false as "not decodable", not as an empty
// field.
func decodeIndexField(b []byte, offset int) (field []byte, next int, ok bool) {
	if offset < 0 || offset+2 > len(b) {
		return nil, 0, false
	}
	length := binary.BigEndian.Uint16(b[offset : offset+2])
	start := offset + 2
	end := start + int(length)
	if end > len(b) {
		return nil, 0, false
	}
	return b[start:end], end, true
}
