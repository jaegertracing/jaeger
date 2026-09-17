// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/testutils"
)

func TestNormalizeTraceID(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		want      string
		wantError bool
	}{
		{
			name:  "canonical trace ID",
			input: "2be38093ead7a083",
			want:  "2be38093ead7a083",
		},
		{
			name:  "uppercase trace ID",
			input: "2BE38093EAD7A083",
			want:  "2be38093ead7a083",
		},
		{
			name:  "trace ID with leading zeroes",
			input: "00000000000000002be38093ead7a083",
			want:  "2be38093ead7a083",
		},
		{
			name:      "invalid trace ID",
			input:     "not-a-trace-id",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeTraceID(tt.input)

			if tt.wantError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
