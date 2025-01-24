// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package kafka

import (
	"bytes"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"

	"github.com/jaegertracing/jaeger-idl/model/v1"
)

// Marshaller encodes a span into a byte array to be sent to Kafka
type Marshaller interface {
	Marshal(*model.Span) ([]byte, error)
}

type protobufMarshaller struct{}

func newProtobufMarshaller() *protobufMarshaller {
	return &protobufMarshaller{}
}

// Marshal encodes a span as a protobuf byte array
func (*protobufMarshaller) Marshal(span *model.Span) ([]byte, error) {
	return proto.Marshal(span)
}

type jsonMarshaller struct {
	pbMarshaller *jsonpb.Marshaler
}

func newJSONMarshaller() *jsonMarshaller {
	return &jsonMarshaller{&jsonpb.Marshaler{}}
}

// Marshal encodes a span as a json byte array
func (h *jsonMarshaller) Marshal(span *model.Span) ([]byte, error) {
	out := new(bytes.Buffer)
	err := h.pbMarshaller.Marshal(out, span)
	return out.Bytes(), err
}
