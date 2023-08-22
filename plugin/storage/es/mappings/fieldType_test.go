// Copyright (c) 2023 The Jaeger Authors.
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

package mappings

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseFieldType(t *testing.T) {
	tests := []struct {
		name      string
		fieldType any
		expected  FieldType
	}{
		{
			fieldType: "nested",
			expected:  NestedFieldType,
			name:      "parse string nested as NestedFieldType",
		},
		{
			fieldType: "false",
			expected:  NestedFieldType,
			name:      "parse string nested as NestedFieldType",
		},
		{
			fieldType: false,
			expected:  NestedFieldType,
			name:      "parse bool false as NestedFieldType",
		},
		{
			fieldType: true,
			expected:  ObjectFieldType,
			name:      "parse bool true as ObjectFieldType",
		},
		{
			fieldType: "true",
			expected:  ObjectFieldType,
			name:      "parse string nested as NestedFieldType",
		},
		{
			fieldType: "object",
			expected:  ObjectFieldType,
			name:      "parse string object as ObjectFieldType",
		},
		{
			fieldType: 12,
			expected:  NestedFieldType,
			name:      "parse any other type as NestedFieldType",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual := ParseFieldType(test.fieldType)
			assert.Equal(t, test.expected, actual)
		})
	}
}

func TestFormat(t *testing.T) {
	tests := []struct {
		fieldType FieldType
		expected  string
		format    string
	}{
		{
			fieldType: NestedFieldType,
			expected:  "0",
			format:    "%s",
		},
		{
			fieldType: ObjectFieldType,
			expected:  "1",
			format:    "%s",
		},
		{
			fieldType: ObjectFieldType,
			expected:  "object",
			format:    "%v",
		},
		{
			fieldType: NestedFieldType,
			expected:  "nested",
			format:    "%v",
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("should be able to format with specifier %s for field type %v", test.format, test.fieldType), func(t *testing.T) {
			actual := fmt.Sprintf(test.format, test.fieldType)
			assert.Equal(t, test.expected, actual)
		})
	}
}
