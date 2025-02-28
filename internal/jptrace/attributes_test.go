// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jptrace

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

func TestAttributesToMap(t *testing.T) {
	tests := []struct {
		name       string
		attributes pcommon.Map
		expected   map[string]string
	}{
		{
			name:       "empty attributes",
			attributes: pcommon.NewMap(),
			expected:   map[string]string{},
		},
		{
			name: "single attribute",
			attributes: func() pcommon.Map {
				m := pcommon.NewMap()
				m.PutStr("key1", "value1")
				return m
			}(),
			expected: map[string]string{"key1": "value1"},
		},
		{
			name: "multiple attributes",
			attributes: func() pcommon.Map {
				m := pcommon.NewMap()
				m.PutStr("key1", "value1")
				m.PutStr("key2", "value2")
				return m
			}(),
			expected: map[string]string{"key1": "value1", "key2": "value2"},
		},
		{
			name: "non-string attributes",
			attributes: func() pcommon.Map {
				m := pcommon.NewMap()
				m.PutInt("key1", 1)
				m.PutDouble("key2", 3.14)
				return m
			}(),
			expected: map[string]string{"key1": "1", "key2": "3.14"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := AttributesToMap(test.attributes)
			require.Equal(t, test.expected, result)
		})
	}
}

func TestMapToAttributes(t *testing.T) {
	tests := []struct {
		name      string
		tags      map[string]string
		requireFn func(t *testing.T, result pcommon.Map)
	}{
		{
			name: "empty map",
			tags: map[string]string{},
			requireFn: func(t *testing.T, result pcommon.Map) {
				require.Equal(t, 0, result.Len(), "Expected map to be empty")
			},
		},
		{
			name: "single tag",
			tags: map[string]string{"key1": "value1"},
			requireFn: func(t *testing.T, result pcommon.Map) {
				val, exists := result.Get("key1")
				require.True(t, exists)
				require.Equal(t, "value1", val.Str())
			},
		},
		{
			name: "multiple tags",
			tags: map[string]string{"key1": "value1", "key2": "value2"},
			requireFn: func(t *testing.T, result pcommon.Map) {
				val, exists := result.Get("key1")
				require.True(t, exists)
				require.Equal(t, "value1", val.Str())

				val, exists = result.Get("key2")
				require.True(t, exists)
				require.Equal(t, "value2", val.Str())
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := MapToAttributes(test.tags)
			test.requireFn(t, result)
		})
	}
}
