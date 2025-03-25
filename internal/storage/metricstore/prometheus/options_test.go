// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package prometheus

import (
	"errors"
	"strconv"
	"testing"

	"github.com/jaegertracing/jaeger/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCLI(t *testing.T) {
	opts := NewOptions()
	v, command := config.Viperize(opts.AddFlags)
	err := command.ParseFlags([]string{
		"--prometheus.query.extra-query-params=key1=value1",
	})
	require.NoError(t, err)

	err = opts.InitFromViper(v)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"key1": "value1"}, opts.ExtraQueryParams)
}

func TestCLIError(t *testing.T) {
	opts := NewOptions()
	v, command := config.Viperize(opts.AddFlags)

	err := command.ParseFlags([]string{
		"--prometheus.query.extra-query-params=key1",
	})
	require.NoError(t, err)

	err = opts.InitFromViper(v)
	require.ErrorContains(t, err, "failed to parse extra query params: failed to parse 'key1'. Expected format: 'param1=value1,param2=value2'")
}

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
