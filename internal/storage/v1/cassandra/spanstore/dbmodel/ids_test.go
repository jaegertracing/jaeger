// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTraceIDJSONRoundTrip(t *testing.T) {
	original := TraceID{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}
	b, err := json.Marshal(original)
	require.NoError(t, err)
	expectedStr := "\"AAAAAAAAAAAAAAAAAAAAAQ==\""
	assert.Equal(t, expectedStr, string(b))
	var decoded TraceID
	err = json.Unmarshal(b, &decoded)
	require.NoError(t, err)
	assert.Equal(t, original, decoded)
}

func TestTraceIDJSONUnmarshal_Errors(t *testing.T) {
	tests := []struct {
		name  string
		input string
		err   string
	}{
		{
			name:  "not a JSON string",
			input: `123`,
			err:   "json: cannot unmarshal number into Go value of type string",
		},
		{
			name:  "invalid base64",
			input: `"@@@@"`,
			err:   "illegal base64 data at input byte 0",
		},
		{
			name:  "invalid decoded length",
			input: `"AQ=="`,
			err:   "invalid TraceID length: 1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var traceId TraceID
			err := json.Unmarshal([]byte(tt.input), &traceId)
			assert.ErrorContains(t, err, tt.err)
		})
	}
}
