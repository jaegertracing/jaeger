// Copyright (c) 2019 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package dbmodel

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/model"
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
		assert.Equal(t, actualKVs, expectedKVs)

		actualKVs = tf.FilterLogFields(nil, inputKVs)
		actualKVs.Sort()
		assert.Equal(t, actualKVs, expectedKVs)

		actualKVs = tf.FilterProcessTags(nil, inputKVs)
		actualKVs.Sort()
		assert.Equal(t, actualKVs, expectedKVs)

		actualKVs = tf.FilterTags(nil, inputKVs)
		actualKVs.Sort()
		assert.Equal(t, actualKVs, expectedKVs)
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
		assert.Equal(t, actualKVs, expectedKVs)

		actualKVs = tf.FilterLogFields(nil, inputKVs)
		actualKVs.Sort()
		assert.Equal(t, actualKVs, expectedKVs)

		actualKVs = tf.FilterProcessTags(nil, inputKVs)
		actualKVs.Sort()
		assert.Equal(t, actualKVs, expectedKVs)

		actualKVs = tf.FilterTags(nil, inputKVs)
		actualKVs.Sort()
		assert.Equal(t, actualKVs, expectedKVs)
	}
}
