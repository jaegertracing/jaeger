// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
				warnings := attrs.PutEmptySlice("jaeger.adjuster.warning")
				for _, warn := range test.existing {
					warnings.AppendEmpty().SetStr(warn)
				}
			}
			addWarning(span, test.newWarn)
			warnings, ok := attrs.Get("jaeger.adjuster.warning")
			assert.True(t, ok)
			assert.Equal(t, len(test.expected), warnings.Slice().Len())
			for i, expectedWarn := range test.expected {
				assert.Equal(t, expectedWarn, warnings.Slice().At(i).Str())
			}
		})
	}
}
