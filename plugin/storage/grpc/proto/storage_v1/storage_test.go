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

package storage_v1_test

import (
	"bytes"
	"testing"

	"github.com/jaegertracing/jaeger/plugin/storage/grpc/proto/storageprototest"

	"github.com/jaegertracing/jaeger/plugin/storage/grpc/proto/storage_v1"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"
	"github.com/jaegertracing/jaeger/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetTraceRequestMarshalProto(t *testing.T) {
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
			expected: `{"traceId":"AAAAAAAAAAIAAAAAAAAAAw=="}`,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ref1 := storage_v1.GetTraceRequest{TraceID: model.NewTraceID(2, 3)}
			ref2 := storageprototest.GetTraceRequest{
				// TODO: would be cool to fuzz that test
				TraceId: []byte{0, 0, 0, 0, 0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 3},
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
			var ref1u storage_v1.GetTraceRequest
			err = testCase.unmarshal(d2, &ref1u)
			require.NoError(t, err)
			assert.Equal(t, ref1, ref1u)
		})
	}
}

func TestFindTraceIDsResponseMarshalProto(t *testing.T) {
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
			expected: `{"traceIds":["AAAAAAAAAAIAAAAAAAAAAw==","AAAAAAAAAAQAAAAAAAAAAQ=="]}`,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ref1 := storage_v1.FindTraceIDsResponse{TraceIDs: []model.TraceID{
				model.NewTraceID(2, 3),
				model.NewTraceID(4, 1),
			}}
			ref2 := storageprototest.FindTraceIDsResponse{TraceIds: [][]byte{
				// TODO: would be cool to fuzz that test
				[]byte{0, 0, 0, 0, 0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 3},
				[]byte{0, 0, 0, 0, 0, 0, 0, 4, 0, 0, 0, 0, 0, 0, 0, 1},
			}}
			d1, err := testCase.marshal(&ref1)
			require.NoError(t, err)
			d2, err := testCase.marshal(&ref2)
			require.NoError(t, err)
			assert.Equal(t, d2, d1)
			if testCase.expected != "" {
				assert.Equal(t, testCase.expected, string(d1))
			}
			// test unmarshal
			var ref1u storage_v1.FindTraceIDsResponse
			err = testCase.unmarshal(d2, &ref1u)
			require.NoError(t, err)
			assert.Equal(t, ref1, ref1u)
		})
	}
}
