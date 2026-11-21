// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

func TestTraceID_ToOTEL(t *testing.T) {
	tests := []struct {
		name     string
		input    TraceID
		expected pcommon.TraceID
		errMsg   string
	}{
		{
			name:  "128-bit trace ID (32 hex chars)",
			input: "0102030405060708090a0b0c0d0e0f10",
			expected: pcommon.TraceID(
				[16]byte{
					0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
					0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
				},
			),
		},
		{
			name:     "invalid hex string",
			input:    "wxyzwxyzwxyzwxyzwxyzwxyzwxyzwxyz",
			expected: pcommon.TraceID{},
			errMsg:   "encoding/hex: invalid byte",
		},
		{
			name:     "trace ID too long",
			input:    "0102030405060708090a0b0c0d0e0f1011",
			expected: pcommon.TraceID{},
			errMsg:   "trace ID from DB is NOT 32 chars",
		},
		{
			name:     "trace ID too short",
			input:    "090a0b0c0d0e0f10",
			expected: pcommon.TraceID{},
			errMsg:   "trace ID from DB is NOT 32 chars",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.input.ToOTEL()
			if tt.errMsg != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, got)
			}
		})
	}
}

func TestSpanID_ToOTEL(t *testing.T) {
	tests := []struct {
		name     string
		input    SpanID
		expected pcommon.SpanID
		errMsg   string
	}{
		{
			name:  "full 64-bit span ID (16 hex chars)",
			input: "0102030405060708",
			expected: pcommon.SpanID(
				[8]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08},
			),
		},
		{
			name:     "invalid hex string",
			input:    "wxyzwxyzwxyzwxyz",
			expected: pcommon.SpanID{},
			errMsg:   "encoding/hex: invalid byte",
		},
		{
			name:     "span ID too long",
			input:    "010203040506070809",
			expected: pcommon.SpanID{},
			errMsg:   "span ID from DB is NOT 16 chars",
		},
		{
			name:     "span ID too short",
			input:    "05060708",
			expected: pcommon.SpanID{},
			errMsg:   "span ID from DB is NOT 16 chars",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.input.ToOTEL()
			if tt.errMsg != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, got)
			}
		})
	}
}
