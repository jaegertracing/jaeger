// Copyright (c) 2019 The Jaeger Authors.
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

package model

import (
	"bytes"
	"testing"

	"github.com/gogo/protobuf/jsonpb"
	proto "github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/model/prototest"
)

func TestDependencyLinkApplyDefaults(t *testing.T) {
	dl := DependencyLink{}.ApplyDefaults()
	assert.Equal(t, JaegerDependencyLinkSource, dl.Source)

	networkSource := DependencyLinkSource("network")
	dl = DependencyLink{Source: networkSource}.ApplyDefaults()
	assert.Equal(t, networkSource, dl.Source)
}

func TestDependencyLinkSourceMarshalProto(t *testing.T) {
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
			expected: `{"parent":"parent","child":"child","callCount":"1","source":"jaeger"}`,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			link1 := DependencyLink{CallCount: 1, Child: "child", Parent: "parent", Source: JaegerDependencyLinkSource}
			link2 := prototest.DependencyLink{CallCount: 1, Child: "child", Parent: "parent", Source: "jaeger"}
			d1, err := testCase.marshal(&link1)
			require.NoError(t, err)
			d2, err := testCase.marshal(&link2)
			require.NoError(t, err)
			assert.Equal(t, d2, d1)
			if testCase.expected != "" {
				assert.Equal(t, testCase.expected, string(d1))
			}
			// test unmarshal
			var link1u DependencyLink
			err = testCase.unmarshal(d2, &link1u)
			require.NoError(t, err)
			assert.Equal(t, link1, link1u)
		})
	}
}
