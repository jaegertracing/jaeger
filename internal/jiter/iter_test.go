// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jiter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectWithErrors(t *testing.T) {
	type item struct {
		str string
		err error
	}
	tests := []struct {
		name     string
		items    []item
		expected []string
		err      error
	}{
		{
			name: "no errors",
			items: []item{
				{str: "a", err: nil},
				{str: "b", err: nil},
				{str: "c", err: nil},
			},
			expected: []string{"a", "b", "c"},
		},
		{
			name: "first error",
			items: []item{
				{str: "a", err: nil},
				{str: "b", err: nil},
				{str: "c", err: assert.AnError},
			},
			err: assert.AnError,
		},
		{
			name: "second error",
			items: []item{
				{str: "a", err: nil},
				{str: "b", err: assert.AnError},
				{str: "c", err: nil},
			},
			err: assert.AnError,
		},
		{
			name: "third error",
			items: []item{
				{str: "a", err: nil},
				{str: "b", err: nil},
				{str: "c", err: assert.AnError},
			},

			err: assert.AnError,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			seq := func(yield func(string, error) bool) {
				for _, item := range test.items {
					if !yield(item.str, item.err) {
						return
					}
				}
			}
			result, err := CollectWithErrors(seq)
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
	type item struct {
		strs []string
		err  error
	}
	tests := []struct {
		name     string
		items    []item
		expected []string
		err      error
	}{
		{
			name: "no errors",
			items: []item{
				{strs: []string{"a", "b", "c"}, err: nil},
				{strs: []string{"d", "e", "f"}, err: nil},
				{strs: []string{"g", "h", "i"}, err: nil},
			},
			expected: []string{"a", "b", "c", "d", "e", "f", "g", "h", "i"},
		},
		{
			name: "first error",
			items: []item{
				{strs: []string{"a", "b", "c"}, err: nil},
				{strs: []string{"d", "e", "f"}, err: assert.AnError},
				{strs: []string{"g", "h", "i"}, err: nil},
			},
			err: assert.AnError,
		},
		{
			name: "second error",
			items: []item{
				{strs: []string{"a", "b", "c"}, err: nil},
				{strs: []string{"d", "e", "f"}, err: nil},
				{strs: []string{"g", "h", "i"}, err: assert.AnError},
			},
			err: assert.AnError,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			seq := func(yield func([]string, error) bool) {
				for _, item := range test.items {
					if !yield(item.strs, item.err) {
						return
					}
				}
			}
			result, err := FlattenWithErrors(seq)
			if test.err != nil {
				require.ErrorIs(t, err, test.err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, test.expected, result)
			}
		})
	}
}
