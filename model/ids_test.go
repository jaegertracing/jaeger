// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2018 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package model_test

import (
	"bytes"
	"testing"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/model/prototest"
)

// TraceID/SpanID fields are defined as bytes in proto, backed by custom types in Go.
// Unfortunately, that means they require manual implementations of proto & json serialization.
// To ensure that it's the same as the default protobuf serialization, file jaeger_test.proto
// contains a copy of SpanRef message without any gogo options. This test file is compiled with
// plain protoc -go_out (without gogo). This test performs proto and JSON marshaling/unmarshaling
// to ensure that the outputs of manual and standard serialization are identical.
func TestTraceSpanIDMarshalProto(t *testing.T) {
	testCases := []struct {
		name      string
		marshal   func(proto.Message) ([]byte, error)
		unmarshal func([]byte, proto.Message) error
		expected  string
	}{
		{
			name:      "protobuf",
			marshal:   proto.Marshal,
			unmarshal: proto.Unmarshal,
		},
		{
			name: "JSON",
			marshal: func(m proto.Message) ([]byte, error) {
				out := new(bytes.Buffer)
				err := new(jsonpb.Marshaler).Marshal(out, m)
				if err != nil {
					return nil, err
				}
				return out.Bytes(), nil
			},
			unmarshal: func(in []byte, m proto.Message) error {
				return jsonpb.Unmarshal(bytes.NewReader(in), m)
			},
			expected: `{"traceId":"AAAAAAAAAAIAAAAAAAAAAw==","spanId":"AAAAAAAAAAs="}`,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ref1 := model.SpanRef{TraceID: model.NewTraceID(2, 3), SpanID: model.NewSpanID(11)}
			ref2 := prototest.SpanRef{
				// TODO: would be cool to fuzz that test
				TraceId: []byte{0, 0, 0, 0, 0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 3},
				SpanId:  []byte{0, 0, 0, 0, 0, 0, 0, 11},
			}
			d1, err := testCase.marshal(&ref1)
			require.NoError(t, err)
			d2, err := testCase.marshal(&ref2)
			require.NoError(t, err)
			assert.Equal(t, d2, d1)
			if testCase.expected != "" {
				assert.Equal(t, testCase.expected, string(d1))
			}
			// test unmarshal
			var ref1u model.SpanRef
			err = testCase.unmarshal(d2, &ref1u)
			require.NoError(t, err)
			assert.Equal(t, ref1, ref1u)
		})
	}
}

func TestSpanIDFromBytes(t *testing.T) {
	tests := []struct {
		data   []byte
		errMsg string
	}{
		{data: []byte{0, 0, 0, 0}, errMsg: "invalid length for SpanID"},
		{data: []byte{0, 0, 0, 0, 0, 0, 0, 13, 0}, errMsg: "invalid length for SpanID"},
	}
	for _, test := range tests {
		_, err := model.SpanIDFromBytes(test.data)
		require.Error(t, err)
		assert.Equal(t, err.Error(), test.errMsg)
	}

	spanID, err := model.SpanIDFromBytes([]byte{0, 0, 0, 0, 0, 0, 0, 13})
	require.NoError(t, err)
	assert.Equal(t, spanID, model.NewSpanID(13))
}

func TestTraceIDFromBytes(t *testing.T) {
	errTests := [][]byte{
		{0, 0, 0, 0, 0, 0, 0, 13, 0, 0, 0, 0, 0, 0, 0, 0, 13},
		{0, 0, 0, 0, 0, 0, 0, 13, 0, 0, 0, 0, 0, 0, 13},
		{0, 0, 0, 0, 0, 0, 13},
	}
	for _, data := range errTests {
		_, err := model.TraceIDFromBytes(data)
		require.Error(t, err)
		assert.Equal(t, err.Error(), "invalid length for TraceID")
	}

	tests := []struct {
		data     []byte
		expected model.TraceID
	}{
		{data: []byte{0, 0, 0, 0, 0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 3}, expected: model.NewTraceID(2, 3)},
		{data: []byte{0, 0, 0, 0, 0, 0, 0, 2}, expected: model.NewTraceID(0, 2)},
	}
	for _, test := range tests {
		traceID, err := model.TraceIDFromBytes(test.data)
		require.NoError(t, err)
		assert.Equal(t, traceID, test.expected)
	}
}
