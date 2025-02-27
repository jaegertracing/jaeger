// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jptrace

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
			assert.Equal(t, test.expected, result)
		})
	}
}

func TestMapToAttributes(t *testing.T) {
	tests := []struct {
		name     string
		tags     map[string]string
		expected pcommon.Map
	}{
		{
			name:     "empty map",
			tags:     map[string]string{},
			expected: pcommon.NewMap(),
		},
		{
			name: "single tag",
			tags: map[string]string{"key1": "value1"},
			expected: func() pcommon.Map {
				m := pcommon.NewMap()
				m.PutStr("key1", "value1")
				return m
			}(),
		},
		{
			name: "multiple tags",
			tags: map[string]string{"key1": "value1", "key2": "value2"},
			expected: func() pcommon.Map {
				m := pcommon.NewMap()
				m.PutStr("key1", "value1")
				m.PutStr("key2", "value2")
				return m
			}(),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := MapToAttributes(test.tags)
			assert.Equal(t, test.expected, result)
		})
	}
}
