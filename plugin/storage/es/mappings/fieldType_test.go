package mappings

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseFieldType(t *testing.T) {
	tests := []struct {
		fieldType string
		expected  FieldType
	}{
		{
			fieldType: "nested",
			expected:  NestedFieldType,
		},
		{
			fieldType: "object",
			expected:  ObjectFieldType,
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("should be able to parse %s field type", test.fieldType), func(t *testing.T) {
			actual := ParseFieldType(test.fieldType)
			assert.Equal(t, test.expected, actual)
		})
	}
}

func TestFormat(t *testing.T) {
	tests := []struct {
		fieldType FieldType
		expected  string
		format string
	}{
		{
			fieldType: NestedFieldType,
			expected:  "0" ,
			format:    "%s",
		},
		{
			fieldType: ObjectFieldType,
			expected:  "1" ,
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
