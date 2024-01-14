// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package model_test

import (
	"bytes"
	"testing"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/model/v2"
	tracev1 "github.com/jaegertracing/jaeger/proto-gen/otel/trace/v1"
)

// TestMarshal ensures that we can marshal and unmarshal a Span using both JSON and Protobuf.
// Since it depends on proto-gen types, it's in the model_test package to avoid circular dependencies.
func TestMarshalSpan(t *testing.T) {
	tests := []struct {
		name string
		span *tracev1.Span
	}{
		{name: "valid IDs", span: &tracev1.Span{
			TraceID: model.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			SpanID:  model.SpanID{1, 2, 3, 4, 5, 6, 7, 8},
		}},
		{name: "invalid IDs", span: &tracev1.Span{
			TraceID: model.TraceID{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			SpanID:  model.SpanID{0, 0, 0, 0, 0, 0, 0, 0},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name+"/Protobuf", func(t *testing.T) {
			data, err := proto.Marshal(tt.span)
			require.NoError(t, err)

			var span tracev1.Span
			require.NoError(t, proto.Unmarshal(data, &span))
			assert.Equal(t, tt.span, &span)
		})
		t.Run(tt.name+"/JSON", func(t *testing.T) {
			var buf bytes.Buffer
			require.NoError(t, new(jsonpb.Marshaler).Marshal(&buf, tt.span))

			var span tracev1.Span
			require.NoError(t, jsonpb.Unmarshal(&buf, &span))
			assert.Equal(t, tt.span, &span)
		})
	}
}
