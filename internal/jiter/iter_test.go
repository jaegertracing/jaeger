// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jiter

import (
	"iter"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectWithErrors(t *testing.T) {
	tests := []struct {
		name     string
		seq      iter.Seq2[string, error]
		expected []string
		err      error
	}{
		{
			name: "no errors",
			seq: func(yield func(string, error) bool) {
				yield("a", nil)
				yield("b", nil)
				yield("c", nil)
			},
			expected: []string{"a", "b", "c"},
		},
		{
			name: "first error",
			seq: func(yield func(string, error) bool) {
				yield("a", nil)
				yield("b", nil)
				yield("c", assert.AnError)
			},
			err: assert.AnError,
		},
		{
			name: "second error",
			seq: func(yield func(string, error) bool) {
				yield("a", nil)
				yield("b", assert.AnError)
				yield("c", nil)
			},
			err: assert.AnError,
		},
		{
			name: "third error",
			seq: func(yield func(string, error) bool) {
				yield("a", nil)
				yield("b", nil)
				yield("c", assert.AnError)
			},
			err: assert.AnError,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := CollectWithErrors(test.seq)
			if test.err != nil {
				require.ErrorIs(t, err, test.err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, test.expected, result)
			}
		})
	}
}

func TestFlattenWithErrors(t *testing.T) {
	tests := []struct {
		name     string
		seq      iter.Seq2[[]string, error]
		expected []string
		err      error
	}{
		{
			name: "no errors",
			seq: func(yield func([]string, error) bool) {
				yield([]string{"a", "b", "c"}, nil)
				yield([]string{"d", "e", "f"}, nil)
				yield([]string{"g", "h", "i"}, nil)
			},
			expected: []string{"a", "b", "c", "d", "e", "f", "g", "h", "i"},
		},
		{
			name: "first error",
			seq: func(yield func([]string, error) bool) {
				yield([]string{"a", "b", "c"}, nil)
				yield([]string{"d", "e", "f"}, assert.AnError)
				yield([]string{"g", "h", "i"}, nil)
			},
			err: assert.AnError,
		},
		{
			name: "second error",
			seq: func(yield func([]string, error) bool) {
				yield([]string{"a", "b", "c"}, nil)
				yield([]string{"d", "e", "f"}, nil)
				yield([]string{"g", "h", "i"}, assert.AnError)
			},
			err: assert.AnError,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := FlattenWithErrors(test.seq)
			if test.err != nil {
				require.ErrorIs(t, err, test.err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, test.expected, result)
			}
		})
	}
}
