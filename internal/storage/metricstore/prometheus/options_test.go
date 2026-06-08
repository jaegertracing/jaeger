// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package prometheus

import (
	"errors"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	jconfig "github.com/jaegertracing/jaeger/internal/config"
	config "github.com/jaegertracing/jaeger/internal/config/promcfg"
)

func TestCLI(t *testing.T) {
	opts := Options{
		Configuration: config.Configuration{
			ExtraQueryParams: map[string]string{"key1": "value1"},
		},
	}

	assert.Equal(t, map[string]string{"key1": "value1"}, opts.ExtraQueryParams)
}

func TestCLIError(t *testing.T) {
	_, err := parseKV("key1")
	assert.ErrorContains(t, err, "failed to parse 'key1'. Expected format: 'param1=value1,param2=value2'")
}

func TestInitFromViperLatencyUnit(t *testing.T) {
	t.Run("invalid unit is rejected", func(t *testing.T) {
		opts := NewOptions()
		v, _ := jconfig.Viperize(opts.AddFlags)
		v.Set(prefix+suffixLatencyUnit, "us")
		err := opts.InitFromViper(v)
		require.EqualError(t, err, `latency_unit must be "ms" or "s", not "us"`)
	})
	t.Run("valid unit is accepted", func(t *testing.T) {
		opts := NewOptions()
		v, _ := jconfig.Viperize(opts.AddFlags)
		v.Set(prefix+suffixLatencyUnit, "s")
		err := opts.InitFromViper(v)
		require.NoError(t, err)
		assert.Equal(t, "s", opts.LatencyUnit)
	})
	t.Run("empty unit is rejected at the flag layer", func(t *testing.T) {
		// Unlike NewFactoryWithConfig (which normalizes empty to the default),
		// the v1 flag path rejects an explicitly empty unit.
		opts := NewOptions()
		v, _ := jconfig.Viperize(opts.AddFlags)
		v.Set(prefix+suffixLatencyUnit, "")
		err := opts.InitFromViper(v)
		require.EqualError(t, err, `latency_unit must be "ms" or "s", not ""`)
	})
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
