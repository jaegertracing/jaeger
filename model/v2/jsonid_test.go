// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package model_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/crossdock/crossdock-go/assert"
	"github.com/gogo/protobuf/jsonpb"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/model/v2"
	tracev1 "github.com/jaegertracing/jaeger/proto-gen/otel/trace/v1"
)

func TestMarshalJSON(t *testing.T) {
	// var spanID SpanID = [spanIDSize]byte{1, 2, 3, 4, 5, 6, 7, 8}
	// var id [spanIDSize]byte = {1, 2, 3, 4, 5, 6, 7, 8}
	s1 := &tracev1.Span{
		TraceID: model.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SpanID:  model.SpanID{1, 2, 3, 4, 5, 6, 7, 8},
	}
	var buf bytes.Buffer
	require.NoError(t, new(jsonpb.Marshaler).Marshal(&buf, s1))

	var s2 tracev1.Span
	require.NoError(t, jsonpb.Unmarshal(&buf, &s2))
	assert.Equal(t, s1, &s2)
}

func TestUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		dst      []byte
		src      []byte
		expected error
	}{
		{
			name:     "Valid input",
			dst:      make([]byte, 16+2),
			src:      []byte(`"AAAAAAAAAJYAAAAAAAAAoA=="`),
			expected: nil,
		},
		{
			name:     "Empty input",
			dst:      make([]byte, 16),
			src:      []byte(`""`),
			expected: nil,
		},
		{
			name:     "Invalid length",
			dst:      make([]byte, 16),
			src:      []byte(`"AAAAAAAAAJYAAAAAAAAAoA=="`),
			expected: fmt.Errorf("invalid length for ID"),
		},
		{
			name:     "Decode error",
			dst:      make([]byte, 16+2),
			src:      []byte(`"invalid_base64_length_18"`),
			expected: fmt.Errorf("cannot unmarshal ID from string"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := model.UnmarshalJSON(tt.dst, tt.src)
			if tt.expected == nil {
				require.NoError(t, err)
			} else {
				assert.Contains(t, err.Error(), tt.expected.Error())
			}
		})
	}
}
