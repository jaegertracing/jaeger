// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name   string
		dst    []byte
		src    []byte
		expErr string
	}{
		{
			name:   "Valid input",
			dst:    make([]byte, 16+2),
			src:    []byte(`"AAAAAAAAAJYAAAAAAAAAoA=="`),
			expErr: "",
		},
		{
			name:   "Empty input",
			dst:    make([]byte, 16),
			src:    []byte(`""`),
			expErr: "",
		},
		{
			name:   "Invalid length",
			dst:    make([]byte, 16),
			src:    []byte(`"AAAAAAAAAJYAAAAAAAAAoA=="`),
			expErr: "invalid length for ID",
		},
		{
			name:   "Decode error",
			dst:    make([]byte, 16+2),
			src:    []byte(`"invalid_base64_length_18"`),
			expErr: "cannot unmarshal ID from string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := unmarshalJSON(tt.dst, tt.src)
			if tt.expErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.expErr)
			}
		})
	}
}

func TestMarshal(t *testing.T) {
	validSpanID := SpanID{1, 2, 3, 4, 5, 6, 7, 8}
	validTraceID := TraceID{1, 2, 3, 4, 5, 6, 7, 8, 8, 7, 6, 5, 4, 3, 2, 1}
	tests := []struct {
		name   string
		id     gogoCustom
		expErr string
	}{
		{
			name: "Valid span id",
			id:   &validSpanID,
		},
		{
			name: "Valid trace id",
			id:   &validTraceID,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, err := tt.id.Marshal()
			require.NoError(t, err)
			assert.Len(t, d, tt.id.Size())
		})
	}
}

func TestMarshalTo(t *testing.T) {
	validSpanID := SpanID{1, 2, 3, 4, 5, 6, 7, 8}
	validTraceID := TraceID{1, 2, 3, 4, 5, 6, 7, 8, 8, 7, 6, 5, 4, 3, 2, 1}
	tests := []struct {
		name   string
		id     gogoCustom
		len    int
		expErr string
	}{
		{
			name: "Valid span id buffer",
			id:   &validSpanID,
			len:  8,
		},
		{
			name: "Valid trace id buffer",
			id:   &validTraceID,
			len:  16,
		},
		{
			name:   "Invalid span id buffer",
			id:     &validSpanID,
			len:    4,
			expErr: errMarshalSpanID.Error(),
		},
		{
			name:   "Invalid trace id buffer",
			id:     &validTraceID,
			len:    4,
			expErr: errMarshalTraceID.Error(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := make([]byte, tt.len)
			n, err := tt.id.MarshalTo(buf)
			if tt.expErr == "" {
				require.NoError(t, err)
				assert.Equal(t, n, tt.id.Size())
			} else {
				require.ErrorContains(t, err, tt.expErr)
			}
		})
	}
}

func TestUnmarshalError(t *testing.T) {
	validSpanID := SpanID{1, 2, 3, 4, 5, 6, 7, 8}
	validTraceID := TraceID{1, 2, 3, 4, 5, 6, 7, 8, 8, 7, 6, 5, 4, 3, 2, 1}
	tests := []struct {
		name string
		id   gogoCustom
	}{
		{
			name: "span id",
			id:   &validSpanID,
		},
		{
			name: "trace id",
			id:   &validTraceID,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Run("Protobuf", func(t *testing.T) {
				err := tt.id.Unmarshal([]byte("invalid"))
				require.ErrorContains(t, err, "length")
			})
			t.Run("JSON", func(t *testing.T) {
				err := tt.id.UnmarshalJSON([]byte("invalid"))
				require.ErrorContains(t, err, "length")
			})
		})
	}
}
