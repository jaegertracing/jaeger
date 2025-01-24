// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"testing"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/stretchr/testify/assert"
)

func TestBlacklistFilter(t *testing.T) {
	tt := []struct {
		input    []string
		filter   []string
		expected []string
	}{
		{
			input:    []string{"a", "b", "c"},
			filter:   []string{"a"},
			expected: []string{"b", "c"},
		},
		{
			input:    []string{"a", "b", "c"},
			filter:   []string{"A"},
			expected: []string{"a", "b", "c"},
		},
	}

	for _, test := range tt {
		var inputKVs model.KeyValues
		for _, i := range test.input {
			inputKVs = append(inputKVs, model.String(i, ""))
		}
		var expectedKVs model.KeyValues
		for _, e := range test.expected {
			expectedKVs = append(expectedKVs, model.String(e, ""))
		}
		expectedKVs.Sort()

		tf := NewBlacklistFilter(test.filter)
		actualKVs := tf.filter(inputKVs)
		actualKVs.Sort()
		assert.Equal(t, expectedKVs, actualKVs)

		actualKVs = tf.FilterLogFields(nil, inputKVs)
		actualKVs.Sort()
		assert.Equal(t, expectedKVs, actualKVs)

		actualKVs = tf.FilterProcessTags(nil, inputKVs)
		actualKVs.Sort()
		assert.Equal(t, expectedKVs, actualKVs)

		actualKVs = tf.FilterTags(nil, inputKVs)
		actualKVs.Sort()
		assert.Equal(t, expectedKVs, actualKVs)
	}
}

func TestWhitelistFilter(t *testing.T) {
	tt := []struct {
		input    []string
		filter   []string
		expected []string
	}{
		{
			input:    []string{"a", "b", "c"},
			filter:   []string{"a"},
			expected: []string{"a"},
		},
		{
			input:    []string{"a", "b", "c"},
			filter:   []string{"A"},
			expected: []string{},
		},
	}

	for _, test := range tt {
		var inputKVs model.KeyValues
		for _, i := range test.input {
			inputKVs = append(inputKVs, model.String(i, ""))
		}
		var expectedKVs model.KeyValues
		for _, e := range test.expected {
			expectedKVs = append(expectedKVs, model.String(e, ""))
		}
		expectedKVs.Sort()

		tf := NewWhitelistFilter(test.filter)
		actualKVs := tf.filter(inputKVs)
		actualKVs.Sort()
		assert.Equal(t, expectedKVs, actualKVs)

		actualKVs = tf.FilterLogFields(nil, inputKVs)
		actualKVs.Sort()
		assert.Equal(t, expectedKVs, actualKVs)

		actualKVs = tf.FilterProcessTags(nil, inputKVs)
		actualKVs.Sort()
		assert.Equal(t, expectedKVs, actualKVs)

		actualKVs = tf.FilterTags(nil, inputKVs)
		actualKVs.Sort()
		assert.Equal(t, expectedKVs, actualKVs)
	}
}
