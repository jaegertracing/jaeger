// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jptrace

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

func TestAttributesToStringMap(t *testing.T) {
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
			result := AttributesToStringMap(test.attributes)
			require.Equal(t, test.expected, result)
		})
	}
}

func TestMapToAttributes(t *testing.T) {
	tests := []struct {
		name string
		tags map[string]string
	}{
		{
			name: "empty map",
			tags: map[string]string{},
		},
		{
			name: "single tag",
			tags: map[string]string{"key1": "value1"},
		},
		{
			name: "multiple tags",
			tags: map[string]string{"key1": "value1", "key2": "value2"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := MapToAttributes(test.tags)
			require.Equal(t, test.tags, AttributesToStringMap(result))
		})
	}
}

func TestAttributesToMap(t *testing.T) {
	tests := []struct {
		name       string
		attributes pcommon.Map
		expected   map[string]any
	}{
		{
			name:       "empty attributes",
			attributes: pcommon.NewMap(),
			expected:   map[string]any{},
		},
		{
			name: "single attribute",
			attributes: func() pcommon.Map {
				m := pcommon.NewMap()
				m.PutStr("key1", "value1")
				return m
			}(),
			expected: map[string]any{"key1": "value1"},
		},
		{
			name: "multiple attributes",
			attributes: func() pcommon.Map {
				m := pcommon.NewMap()
				m.PutStr("key1", "value1")
				m.PutStr("key2", "value2")
				return m
			}(),
			expected: map[string]any{"key1": "value1", "key2": "value2"},
		},
		{
			name: "non-string attributes",
			attributes: func() pcommon.Map {
				m := pcommon.NewMap()
				m.PutInt("key1", 1)
				m.PutDouble("key2", 3.14)
				return m
			}(),
			expected: map[string]any{"key1": int64(1), "key2": 3.14},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := AttributesToMap(test.attributes)
			require.Equal(t, test.expected, result)
		})
	}
}
