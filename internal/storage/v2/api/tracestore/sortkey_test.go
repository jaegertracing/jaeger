// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeSortKey(t *testing.T) {
	key, err := EncodeSortKey([]any{int64(10), "t1"})
	require.NoError(t, err)
	assert.JSONEq(t, `[10,"t1"]`, string(key))

	key, err = EncodeSortKey([]json.RawMessage{json.RawMessage(`10`), json.RawMessage(`"t1"`)})
	require.NoError(t, err)
	assert.JSONEq(t, `[10,"t1"]`, string(key))

	_, err = EncodeSortKey([]json.RawMessage{json.RawMessage(`not json`)})
	require.Error(t, err)
}

func TestDecodeSortKey(t *testing.T) {
	types := []SortKeyValueType{SortKeyInteger, SortKeyString, SortKeyString, SortKeyString}
	valid := []string{`10`, `"t1"`, `"i1"`, `"d1"`}
	keyWith := func(i int, value string) []byte {
		values := append([]string{}, valid...)
		values[i] = value
		return []byte("[" + strings.Join(values, ",") + "]")
	}
	for _, tc := range []struct {
		name string
		key  []byte
		err  string
	}{
		{"NotJSON", []byte("not json"), "does not carry a span position"},
		{"TooShort", []byte(`[10,"t1","i1"]`), "carries 3 values for 4 sort fields"},
		{"TooLong", []byte(`[10,"t1","i1","d1","extra"]`), "carries 5 values for 4 sort fields"},
		{"ObjectForInteger", keyWith(0, `{}`), "value 0 is not of type integer"},
		{"FractionForInteger", keyWith(0, `1.5`), "value 0 is not of type integer"},
		{"NullForInteger", keyWith(0, `null`), "value 0 is not of type integer"},
		{"NullForString", keyWith(1, `null`), "value 1 is not of type string"},
		{"NumberForString", keyWith(2, `7`), "value 2 is not of type string"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodeSortKey(tc.key, types)
			require.ErrorIs(t, err, ErrPaginationInvalid)
			require.ErrorContains(t, err, tc.err)
		})
	}
	t.Run("Valid", func(t *testing.T) {
		key, err := DecodeSortKey(keyWith(0, `10`), types)
		require.NoError(t, err)
		assert.Equal(t, []any{int64(10), "t1", "i1", "d1"}, key)
	})
	t.Run("None", func(t *testing.T) {
		key, err := DecodeSortKey(nil, types)
		require.NoError(t, err)
		assert.Nil(t, key)
	})
	t.Run("UnknownType", func(t *testing.T) {
		_, err := DecodeSortKey([]byte(`[10]`), []SortKeyValueType{SortKeyValueType(99)})
		require.EqualError(t, err, "sort key value type SortKeyValueType(99) has no decoder")
		require.NotErrorIs(t, err, ErrPaginationInvalid)
	})
}
