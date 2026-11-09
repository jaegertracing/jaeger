// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// SortKeyValueType is the Go type of one value of a sort key, decided by the sort column
// the value belongs to: an ID or an index name is a string, a time or a duration is an int64.
type SortKeyValueType int

const (
	SortKeyString SortKeyValueType = iota
	SortKeyInteger
)

func (t SortKeyValueType) String() string {
	switch t {
	case SortKeyString:
		return "string"
	case SortKeyInteger:
		return "integer"
	default:
		return fmt.Sprintf("SortKeyValueType(%d)", int(t))
	}
}

// A sort key is where a page of a span search ended: the values of the last span's sort
// columns, one per term of the effective order, as a JSON array in the order of the terms.
// The next page selects the spans that come after those values in the ORDER BY, instead of
// skipping a count of rows (RFC 0016 §6.4). The key is the cursor a Reader wraps in its
// PageToken, and the term list that built the ORDER BY is what gives each position its
// meaning, so the key carries values only.
//
// EncodeSortKey writes the key. The values are whatever the backend read from its sort columns,
// raw JSON from an Elasticsearch hit or typed Go values from a ClickHouse row.
func EncodeSortKey[T any](values []T) ([]byte, error) {
	return json.Marshal(values)
}

// DecodeSortKey reads a sort key back as one Go value of the given type per position, a
// string or an int64, or returns nil for an empty key. The key is refused unless it holds one
// value per type and each value is of its type, since a Reader never returns one shaped
// otherwise. The three refusals carry distinct messages so that a test of one cannot pass by
// tripping another.
func DecodeSortKey(raw []byte, types []SortKeyValueType) ([]any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var values []json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, fmt.Errorf("%w: page token does not carry a span position", ErrPaginationInvalid)
	}
	if len(values) != len(types) {
		return nil, fmt.Errorf("%w: page token carries %d values for %d sort fields", ErrPaginationInvalid, len(values), len(types))
	}
	key := make([]any, len(values))
	for i, valueType := range types {
		var err error
		switch valueType {
		case SortKeyString:
			var s string
			err = json.Unmarshal(values[i], &s)
			key[i] = s
		case SortKeyInteger:
			var n int64
			err = json.Unmarshal(values[i], &n)
			key[i] = n
		default:
			return nil, fmt.Errorf("sort key value type %s has no decoder", valueType)
		}
		// A JSON null decodes into either target without error, so it is refused on its own.
		if err != nil || bytes.Equal(bytes.TrimSpace(values[i]), []byte("null")) {
			return nil, fmt.Errorf("%w: page token value %d is not of type %s", ErrPaginationInvalid, i, valueType)
		}
	}
	return key, nil
}
