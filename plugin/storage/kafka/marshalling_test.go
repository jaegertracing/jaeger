// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package kafka

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/model/converter/thrift/zipkin"
	"github.com/jaegertracing/jaeger-idl/thrift-gen/zipkincore"
)

func TestProtobufMarshallerAndUnmarshaller(t *testing.T) {
	testMarshallerAndUnmarshaller(t, newProtobufMarshaller(), NewProtobufUnmarshaller())
}

func TestJSONMarshallerAndUnmarshaller(t *testing.T) {
	testMarshallerAndUnmarshaller(t, newJSONMarshaller(), NewJSONUnmarshaller())
}

func testMarshallerAndUnmarshaller(t *testing.T, marshaller Marshaller, unmarshaller Unmarshaller) {
	bytes, err := marshaller.Marshal(sampleSpan)

	require.NoError(t, err)
	assert.NotNil(t, bytes)

	resultSpan, err := unmarshaller.Unmarshal(bytes)

	require.NoError(t, err)
	assert.Equal(t, sampleSpan, resultSpan)
}

func TestZipkinThriftUnmarshaller(t *testing.T) {
	operationName := "foo"
	bytes := zipkin.SerializeThrift(context.Background(), []*zipkincore.Span{
		{
			ID:   12345,
			Name: operationName,
			Annotations: []*zipkincore.Annotation{
				{Host: &zipkincore.Endpoint{ServiceName: "foobar"}},
			},
		},
	})
	unmarshaller := NewZipkinThriftUnmarshaller()
	resultSpan, err := unmarshaller.Unmarshal(bytes)

	require.NoError(t, err)
	assert.Equal(t, operationName, resultSpan.OperationName)
}

func TestZipkinThriftUnmarshallerErrorNoService(t *testing.T) {
	bytes := zipkin.SerializeThrift(context.Background(), []*zipkincore.Span{
		{
			ID:   12345,
			Name: "foo",
		},
	})
	unmarshaller := NewZipkinThriftUnmarshaller()
	_, err := unmarshaller.Unmarshal(bytes)
	require.Error(t, err)
}

func TestZipkinThriftUnmarshallerErrorCorrupted(t *testing.T) {
	bytes := []byte("foo")
	unmarshaller := NewZipkinThriftUnmarshaller()
	_, err := unmarshaller.Unmarshal(bytes)
	require.Error(t, err)
}
