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

// NewJSONUnmarshaller constructs a ProtobufUnmarshaller
func NewJSONUnmarshaller() *JSONUnmarshaller {
	return &JSONUnmarshaller{}
}

// Unmarshal decodes a json byte array to a span
func (h *JSONUnmarshaller) Unmarshal(msg []byte) (*model.Span, error) {
	newSpan := &model.Span{}
	err := jsonpb.Unmarshal(bytes.NewReader(msg), newSpan)
	return newSpan, err
}
