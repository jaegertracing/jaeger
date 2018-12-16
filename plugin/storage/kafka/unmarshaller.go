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
	"bytes"

	"github.com/apache/thrift/lib/go/thrift"
	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"

	"github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
	"github.com/jaegertracing/jaeger/model/converter/thrift/zipkin"
	"github.com/jaegertracing/jaeger/model"
)

// Unmarshaller decodes a byte array to a span
type Unmarshaller interface {
	Unmarshal([]byte) (*model.Span, error)
}

// ProtobufUnmarshaller implements Unmarshaller
type ProtobufUnmarshaller struct{}

// NewProtobufUnmarshaller constructs a ProtobufUnmarshaller
func NewProtobufUnmarshaller() *ProtobufUnmarshaller {
	return &ProtobufUnmarshaller{}
}

// Unmarshal decodes a protobuf byte array to a span
func (h *ProtobufUnmarshaller) Unmarshal(msg []byte) (*model.Span, error) {
	newSpan := &model.Span{}
	err := proto.Unmarshal(msg, newSpan)
	return newSpan, err
}

// JSONUnmarshaller implements Unmarshaller
type JSONUnmarshaller struct{}

// NewJSONUnmarshaller constructs a JSONUnmarshaller
func NewJSONUnmarshaller() *JSONUnmarshaller {
	return &JSONUnmarshaller{}
}

// Unmarshal decodes a json byte array to a span
func (h *JSONUnmarshaller) Unmarshal(msg []byte) (*model.Span, error) {
	newSpan := &model.Span{}
	err := jsonpb.Unmarshal(bytes.NewReader(msg), newSpan)
	return newSpan, err
}

// zipkinThriftUnmarshaller implements Unmarshaller
type zipkinThriftUnmarshaller struct{}

// NewZipkinThriftUnmarshaller constructs a zipkinThriftUnmarshaller
func NewZipkinThriftUnmarshaller() *zipkinThriftUnmarshaller {
	return &zipkinThriftUnmarshaller{}
}

// Unmarshal decodes a json byte array to a span
func (h *zipkinThriftUnmarshaller) Unmarshal(msg []byte) (*model.Span, error) {
	buffer := thrift.NewTMemoryBuffer()
	buffer.Write(msg)

	transport := thrift.NewTBinaryProtocolTransport(buffer)
	_, _, err := transport.ReadListBegin() // Ignore the returned element type
	if err != nil {
		return nil, err
	}

	zs := &zipkincore.Span{}
	if err = zs.Read(transport); err != nil {
		return nil, err
	}
	mSpan, err := zipkin.ToDomainSpan(zs)

	if err != nil {
		return nil, err
	}
	return mSpan[0], err
}
