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
			expected:  nestedFieldType,
		},
		{
			fieldType: "object",
			expected:  objectFieldType,
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
			fieldType: nestedFieldType,
			expected:  "0" ,
			format: "%s",
		},
		{
			fieldType: objectFieldType,
			expected:  "1" ,
			format: "%s",
		},
		{
			fieldType: objectFieldType,
			expected:  "object",
			format: "%v",
		},
		{
			fieldType: nestedFieldType,
			expected:  "nested",
			format: "%v",
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("should be able to format with specifier %s for field type %v", test.format, test.fieldType), func(t *testing.T) {
			actual := fmt.Sprintf(test.format, test.fieldType)
			assert.Equal(t, test.expected, actual)
		})
	}
}
