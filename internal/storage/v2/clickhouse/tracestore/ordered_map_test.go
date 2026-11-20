// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestOrderedMap_Iterator pins what the driver reads from an orderedMap: the entries in their
// order, with a key that repeats kept.
func TestOrderedMap_Iterator(t *testing.T) {
	m := orderedMap[int64]{
		keys:   []string{"z", "a", "z"},
		values: []int64{1, 2, 3},
	}
	m.Put("ignored", int64(4))

	var keys []string
	var values []int64
	for it := m.Iterator(); it.Next(); {
		keys = append(keys, it.Key().(string))
		values = append(values, it.Value().(int64))
	}

	require.Equal(t, m.keys, keys)
	require.Equal(t, m.values, values)
	require.False(t, orderedMap[string]{}.Iterator().Next())
}
