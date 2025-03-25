// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package prometheus

import (
	"errors"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseKV(t *testing.T) {
	tests := []struct {
		input    string
		expected map[string]string
		err      error
	}{
		{
			input:    "",
			expected: map[string]string{},
			err:      nil,
		},
		{
			input:    "key1=value1",
			expected: map[string]string{"key1": "value1"},
			err:      nil,
		},
		{
			input:    "key1=value1,key2=value2",
			expected: map[string]string{"key1": "value1", "key2": "value2"},
			err:      nil,
		},
		{
			input:    "key1=value1,key2",
			expected: map[string]string{},
			err:      errors.New("failed to parse 'key1=value1,key2'. Expected format: 'param1=value1,param2=value2'"),
		},
	}

	for i, test := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			kv, err := parseKV(test.input)
			assert.Equal(t, test.expected, kv)
			assert.Equal(t, test.err, err)
		})
	}
}
