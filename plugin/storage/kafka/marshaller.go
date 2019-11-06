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

	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"

	"github.com/jaegertracing/jaeger/model"
)

// Marshaller encodes a span into a byte array to be sent to Kafka
type Marshaller interface {
	Marshal(*model.Span) ([]byte, error)
}

type ProtobufMarshaller struct{}

func NewProtobufMarshaller() *ProtobufMarshaller {
	return &ProtobufMarshaller{}
}

// Marshall encodes a span as a protobuf byte array
func (h *ProtobufMarshaller) Marshal(span *model.Span) ([]byte, error) {
	return proto.Marshal(span)
}

type JsonMarshaller struct {
	pbMarshaller *jsonpb.Marshaler
}

func NewJSONMarshaller() *JsonMarshaller {
	return &JsonMarshaller{&jsonpb.Marshaler{}}
}

// Marshall encodes a span as a json byte array
func (h *JsonMarshaller) Marshal(span *model.Span) ([]byte, error) {
	out := new(bytes.Buffer)
	err := h.pbMarshaller.Marshal(out, span)
	return out.Bytes(), err
}
