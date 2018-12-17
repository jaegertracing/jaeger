// Copyright (c) 2018 The Jaeger Authors.
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

package kafka

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
	"github.com/jaegertracing/jaeger/model/converter/thrift/zipkin"
)

func TestProtobufMarshallerAndUnmarshaller(t *testing.T) {
	testMarshallerAndUnmarshaller(t, newProtobufMarshaller(), NewProtobufUnmarshaller())
}

func TestJSONMarshallerAndUnmarshaller(t *testing.T) {
	testMarshallerAndUnmarshaller(t, newJSONMarshaller(), NewJSONUnmarshaller())
}

func testMarshallerAndUnmarshaller(t *testing.T, marshaller Marshaller, unmarshaller Unmarshaller) {
	bytes, err := marshaller.Marshal(sampleSpan)

	assert.NoError(t, err)
	assert.NotNil(t, bytes)

	resultSpan, err := unmarshaller.Unmarshal(bytes)

	assert.NoError(t, err)
	assert.Equal(t, sampleSpan, resultSpan)
}

func TestZipkinThriftUnmarshaller(t *testing.T) {
	bytes := zipkin.ZipkinSerialize([]*zipkincore.Span{
		{
			ID: 12345,
			Name: "foo",
			Annotations: []*zipkincore.Annotation{
				{Host: &zipkincore.Endpoint{ServiceName: "foo"}},
			},
		},
	})
	unmarshaller := NewZipkinThriftUnmarshaller()
	_, err := unmarshaller.Unmarshal(bytes)
	assert.NoError(t, err)
}

func TestZipkinThriftUnmarshallerErrorNoService(t *testing.T) {
	bytes := zipkin.ZipkinSerialize([]*zipkincore.Span{
		{
			ID: 12345,
			Name: "foo",
		},
	})
	unmarshaller := NewZipkinThriftUnmarshaller()
	_, err := unmarshaller.Unmarshal(bytes)
	assert.Error(t, err)
}

func TestZipkinThriftUnmarshallerErrorCorrupted(t *testing.T) {
	bytes := []byte("foo")
	unmarshaller := NewZipkinThriftUnmarshaller()
	_, err := unmarshaller.Unmarshal(bytes)
	assert.Error(t, err)
}
