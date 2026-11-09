// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

func defaultSpanSort(t *testing.T) []sortTerm {
	t.Helper()
	_, sort, err := spanSort(nil)
	require.NoError(t, err)
	return sort
}

func TestSpanCursor_Encoding(t *testing.T) {
	row := spanSearchRow("a", "0000000000000001", now)
	cursor := newSpanCursor(row, defaultSpanSort(t))
	assert.Equal(t, spanCursor{now.UnixNano(), traceIDHex1, "0000000000000001"}, cursor)
	decoded, err := decodeSpanCursor(cursor.encode(), defaultSpanSort(t))
	require.NoError(t, err)
	assert.Equal(t, cursor, decoded)

	// A cursor of a one-term order carries one value.
	cursor = spanCursor{int64(5)}
	decoded, err = decodeSpanCursor(cursor.encode(), []sortTerm{{column: columnDuration}})
	require.NoError(t, err)
	assert.Equal(t, cursor, decoded)
}

// TestSpanCursor_Refusal shows the cursor is decoded against the types of its terms: a value
// that is not an integer is refused for a numeric column.
func TestSpanCursor_Refusal(t *testing.T) {
	_, err := decodeSpanCursor([]byte(`[1.5]`), []sortTerm{{column: columnDuration}})
	require.ErrorIs(t, err, tracestore.ErrPaginationInvalid)
	require.ErrorContains(t, err, "value 0 is not of type integer")
}
