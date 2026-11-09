// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func defaultSpanSort(t *testing.T) []sortTerm {
	t.Helper()
	sort, err := spanSort(nil)
	require.NoError(t, err)
	return sort
}

func TestSpanCursor_Encoding(t *testing.T) {
	row := spanSearchRow("a", "0000000000000001", now)
	cursor := newSpanCursor(row, defaultSpanSort(t))
	assert.Equal(t, []any{now.UnixNano(), traceIDHex1, "0000000000000001"}, cursor.Key)
	decoded, err := decodeSpanCursor(cursor.encode(), defaultSpanSort(t))
	require.NoError(t, err)
	assert.Equal(t, cursor, decoded)

	// A cursor of a one-term order carries one value.
	cursor = spanCursor{Key: []any{int64(5)}}
	decoded, err = decodeSpanCursor(cursor.encode(), []sortTerm{{column: "duration"}})
	require.NoError(t, err)
	assert.Equal(t, cursor, decoded)
}
