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

func TestWhitelistFilter(t *testing.T) {
	// expected
	tt := [][][]string{
		{
			{"a", "b", "c"}, // input
			{"a"},           // filter
			{"a"},           // expected
		},
		{
			{"a", "b", "c"},
			{"A"},
			{},
		},
	}

	for _, test := range tt {
		input := test[0]
		filter := test[1]
		expected := test[2]

		var inputKVs model.KeyValues
		for _, i := range input {
			inputKVs = append(inputKVs, model.String(i, ""))
		}
		var expectedKVs model.KeyValues
		for _, e := range expected {
			expectedKVs = append(expectedKVs, model.String(e, ""))
		}
		expectedKVs.Sort()

		tf := NewWhitelistTagFilter(filter)
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
