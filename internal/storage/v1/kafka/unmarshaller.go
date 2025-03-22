// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package kafka

import (
	"bytes"
	"context"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"
	zipkin "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/zipkin/zipkinthriftconverter"

	"github.com/jaegertracing/jaeger-idl/model/v1"
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
func (*ProtobufUnmarshaller) Unmarshal(msg []byte) (*model.Span, error) {
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
func (*JSONUnmarshaller) Unmarshal(msg []byte) (*model.Span, error) {
	newSpan := &model.Span{}
	err := jsonpb.Unmarshal(bytes.NewReader(msg), newSpan)
	return newSpan, err
}

// ZipkinThriftUnmarshaller implements Unmarshaller
type ZipkinThriftUnmarshaller struct{}

// NewZipkinThriftUnmarshaller constructs a zipkinThriftUnmarshaller
func NewZipkinThriftUnmarshaller() *ZipkinThriftUnmarshaller {
	return &ZipkinThriftUnmarshaller{}
}

// Unmarshal decodes a json byte array to a span
func (*ZipkinThriftUnmarshaller) Unmarshal(msg []byte) (*model.Span, error) {
	tSpans, err := zipkin.DeserializeThrift(context.Background(), msg)
	if err != nil {
		return nil, err
	}
	mSpans, err := zipkin.ToDomainSpan(tSpans[0])
	if err != nil {
		return nil, err
	}
	return mSpans[0], err
}
