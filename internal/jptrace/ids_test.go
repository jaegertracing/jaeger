// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jptrace

import (
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
			wantErr: `invalid length 0 of decoded trace ID "", expected 16 bytes`,
		},
		{
			name:    "too short",
			input:   "0001",
			wantErr: `invalid length 2 of decoded trace ID "0001", expected 16 bytes`,
		},
		{
			name:    "too long",
			input:   "000100010001000100010001000100010001",
			wantErr: `invalid length 18 of decoded trace ID "000100010001000100010001000100010001", expected 16 bytes`,
		},
		{
			name:    "odd length",
			input:   "abc",
			wantErr: "encoding/hex: odd length hex string",
		},
		{
			name:    "non-hex characters",
			input:   "ZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZ",
			wantErr: "encoding/hex: invalid byte: U+005A 'Z'",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := TraceIDFromString(tt.input)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
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
			wantErr: `invalid length 0 of decoded span ID "", expected 8 bytes`,
		},
		{
			name:    "too short",
			input:   "0001",
			wantErr: `invalid length 2 of decoded span ID "0001", expected 8 bytes`,
		},
		{
			name:    "too long",
			input:   "000100010001000100",
			wantErr: `invalid length 9 of decoded span ID "000100010001000100", expected 8 bytes`,
		},
		{
			name:    "non-hex characters",
			input:   "ZZZZZZZZZZZZZZZZ",
			wantErr: "encoding/hex: invalid byte: U+005A 'Z'",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SpanIDFromString(tt.input)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				assert.Equal(t, pcommon.SpanID{}, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
