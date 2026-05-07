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

func TestGetWarnings_EmptySpan(t *testing.T) {
	span := ptrace.NewSpan()
	span.Attributes().PutStr(WarningsAttribute, "warning-1")
	actual := GetWarnings(span)
	assert.Equal(t, []string{"warning-1"}, actual)
}

// TestAddWarning_NonSliceAttribute verifies that AddWarnings does not panic when
// the WarningsAttribute already exists but is stored as a non-slice type (e.g. a
// plain string written by Elasticsearch or another backend). The existing value
// must be preserved as the first element of the upgraded slice.
func TestAddWarning_NonSliceAttribute(t *testing.T) {
	span := ptrace.NewSpan()
	// Simulate malformed data: attribute exists but is a string, not a slice.
	span.Attributes().PutStr(WarningsAttribute, "pre-existing string warning")
	// Must not panic, and must preserve the existing string value.
	require.NotPanics(t, func() {
		AddWarnings(span, "new warning")
	})
	warnings, ok := span.Attributes().Get(WarningsAttribute)
	require.True(t, ok)
	// The existing string is upgraded to the first element; new warning appended.
	require.Equal(t, 2, warnings.Slice().Len())
	assert.Equal(t, "pre-existing string warning", warnings.Slice().At(0).Str())
	assert.Equal(t, "new warning", warnings.Slice().At(1).Str())
}
