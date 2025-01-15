// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jptrace

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func TestAddWarning(t *testing.T) {
	tests := []struct {
		name     string
		existing []string
		newWarn  string
		expected []string
	}{
		{
			name:     "add to nil warnings",
			existing: nil,
			newWarn:  "new warning",
			expected: []string{"new warning"},
		},
		{
			name:     "add to empty warnings",
			existing: []string{},
			newWarn:  "new warning",
			expected: []string{"new warning"},
		},
		{
			name:     "add to existing warnings",
			existing: []string{"existing warning 1", "existing warning 2"},
			newWarn:  "new warning",
			expected: []string{"existing warning 1", "existing warning 2", "new warning"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			span := ptrace.NewSpan()
			attrs := span.Attributes()
			if test.existing != nil {
				warnings := attrs.PutEmptySlice(WarningsAttribute)
				for _, warn := range test.existing {
					warnings.AppendEmpty().SetStr(warn)
				}
			}
			AddWarnings(span, test.newWarn)
			warnings, ok := attrs.Get(WarningsAttribute)
			assert.True(t, ok)
			assert.Equal(t, len(test.expected), warnings.Slice().Len())
			for i, expectedWarn := range test.expected {
				assert.Equal(t, expectedWarn, warnings.Slice().At(i).Str())
			}
		})
	}
}

func TestAddWarning_MultipleWarnings(t *testing.T) {
	span := ptrace.NewSpan()
	AddWarnings(span, "warning-1", "warning-2")
	warnings, ok := span.Attributes().Get(WarningsAttribute)
	require.True(t, ok)
	require.Equal(t, "warning-1", warnings.Slice().At(0).Str())
	require.Equal(t, "warning-2", warnings.Slice().At(1).Str())
}

func TestGetWarnings(t *testing.T) {
	tests := []struct {
		name     string
		existing []string
		expected []string
	}{
		{
			name:     "get from nil warnings",
			existing: nil,
			expected: nil,
		},
		{
			name:     "get from empty warnings",
			existing: []string{},
			expected: []string{},
		},
		{
			name:     "get from existing warnings",
			existing: []string{"existing warning 1", "existing warning 2"},
			expected: []string{"existing warning 1", "existing warning 2"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			span := ptrace.NewSpan()
			attrs := span.Attributes()
			if test.existing != nil {
				warnings := attrs.PutEmptySlice(WarningsAttribute)
				for _, warn := range test.existing {
					warnings.AppendEmpty().SetStr(warn)
				}
			}
			actual := GetWarnings(span)
			assert.Equal(t, test.expected, actual)
		})
	}
}
