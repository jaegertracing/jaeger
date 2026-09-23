// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jptrace

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

func TestTraceIDFromString(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    pcommon.TraceID
		wantErr string
		// wantHexErr expects the stdlib decoding error to be wrapped, not replaced.
		wantHexErr bool
	}{
		{
			name:  "valid",
			input: "000102030405060708090a0b0c0d0e0f",
			want:  pcommon.TraceID{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
		},
		{
			name:  "uppercase hex",
			input: "000102030405060708090A0B0C0D0E0F",
			want:  pcommon.TraceID{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
		},
		{
			name:  "all zero is accepted",
			input: "00000000000000000000000000000000",
			want:  pcommon.TraceID{},
		},
		{
			name:    "empty",
			input:   "",
			wantErr: `trace ID must be 32 hex characters, got 0`,
		},
		{
			name:    "too short",
			input:   "0001",
			wantErr: `trace ID must be 32 hex characters, got 4`,
		},
		{
			name:    "too long",
			input:   "000100010001000100010001000100010001",
			wantErr: `trace ID must be 32 hex characters, got 36`,
		},
		{
			name:    "odd length",
			input:   "abc",
			wantErr: `trace ID must be 32 hex characters, got 3`,
		},
		{
			name:       "non-hex characters",
			input:      "ZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZ",
			wantErr:    `trace ID "ZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZ" is not valid hex`,
			wantHexErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := TraceIDFromString(tt.input)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				if tt.wantHexErr {
					require.ErrorAs(t, err, new(hex.InvalidByteError))
				}
				assert.Equal(t, pcommon.TraceID{}, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSpanIDFromString(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    pcommon.SpanID
		wantErr string
		// wantHexErr expects the stdlib decoding error to be wrapped, not replaced.
		wantHexErr bool
	}{
		{
			name:  "valid",
			input: "0001020304050607",
			want:  pcommon.SpanID{0, 1, 2, 3, 4, 5, 6, 7},
		},
		{
			name:  "all zero is accepted",
			input: "0000000000000000",
			want:  pcommon.SpanID{},
		},
		{
			name:    "empty",
			input:   "",
			wantErr: `span ID must be 16 hex characters, got 0`,
		},
		{
			name:    "too short",
			input:   "0001",
			wantErr: `span ID must be 16 hex characters, got 4`,
		},
		{
			name:    "too long",
			input:   "000100010001000100",
			wantErr: `span ID must be 16 hex characters, got 18`,
		},
		{
			name:       "non-hex characters",
			input:      "ZZZZZZZZZZZZZZZZ",
			wantErr:    `span ID "ZZZZZZZZZZZZZZZZ" is not valid hex`,
			wantHexErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SpanIDFromString(tt.input)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				if tt.wantHexErr {
					require.ErrorAs(t, err, new(hex.InvalidByteError))
				}
				assert.Equal(t, pcommon.SpanID{}, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
