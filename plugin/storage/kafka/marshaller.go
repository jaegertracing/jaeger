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
	"encoding/json"

	"github.com/apache/thrift/lib/go/thrift"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/model/converter/thrift/jaeger"
)

// Marshaller encodes a span into a byte array to be sent to Kafka
type Marshaller interface {
	Marshal(*model.Span) ([]byte, error)
}

type thriftMarshaller struct {
	tProtocolFactory *thrift.TBinaryProtocolFactory
}

func newThriftMarshaller() *thriftMarshaller {
	return &thriftMarshaller{thrift.NewTBinaryProtocolFactoryDefault()}
}

// Marshall encodes a span as a thrift byte array
func (h *thriftMarshaller) Marshal(span *model.Span) ([]byte, error) {
	thriftSpan := jaeger.FromDomainSpan(span)

	memBuffer := thrift.NewTMemoryBuffer()
	thriftSpan.Write(h.tProtocolFactory.GetProtocol(memBuffer))

	return memBuffer.Bytes(), nil
}

type jsonMarshaller struct{}

func newJSONMarshaller() *jsonMarshaller {
	return &jsonMarshaller{}
}

// Marshall encodes a span as a json byte array
func (h *jsonMarshaller) Marshal(span *model.Span) ([]byte, error) {
	return json.Marshal(span)
}
