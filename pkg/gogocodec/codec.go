// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package gogocodec

import (
	"reflect"
	"strings"

	"github.com/gogo/protobuf/jsonpb"
	gogoproto "github.com/gogo/protobuf/proto"
	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/encoding/proto"
	"google.golang.org/grpc/mem"
)

const (
	jaegerProtoGenPkgPath = "github.com/jaegertracing/jaeger/proto-gen"
	jaegerModelPkgPath    = "github.com/jaegertracing/jaeger/model"
)

var defaultCodec encoding.CodecV2

// CustomType is an interface that Gogo expects custom types to implement.
// https://github.com/gogo/protobuf/blob/master/custom_types.md
type CustomType interface {
	Marshal() ([]byte, error)
	MarshalTo(data []byte) (n int, err error)
	Unmarshal(data []byte) error

	gogoproto.Sizer

	jsonpb.JSONPBMarshaler
	jsonpb.JSONPBUnmarshaler
}

func init() {
	defaultCodec = encoding.GetCodecV2(proto.Name)
	defaultCodec.Name() // ensure it's not nil
	encoding.RegisterCodecV2(newCodec())
}

// gogoCodec forces the use of gogo proto marshalling/unmarshalling for
// Jaeger proto types (package jaeger/gen-proto).
type gogoCodec struct{}

var _ encoding.CodecV2 = (*gogoCodec)(nil)

func newCodec() *gogoCodec {
	return &gogoCodec{}
}

// Name implements encoding.Codec
func (*gogoCodec) Name() string {
	return proto.Name
}

// Marshal implements encoding.Codec
func (*gogoCodec) Marshal(v any) (mem.BufferSlice, error) {
	t := reflect.TypeOf(v)
	elem := t.Elem()
	// use gogo proto only for Jaeger types
	if useGogo(elem) {
		bytes, err := gogoproto.Marshal(v.(gogoproto.Message))
		return mem.BufferSlice{mem.SliceBuffer(bytes)}, err
	}
	return defaultCodec.Marshal(v)
}

// Unmarshal implements encoding.Codec
func (*gogoCodec) Unmarshal(data mem.BufferSlice, v any) error {
	t := reflect.TypeOf(v)
	elem := t.Elem() // only for collections
	// use gogo proto only for Jaeger types
	if useGogo(elem) {
		return gogoproto.Unmarshal(data.Materialize(), v.(gogoproto.Message))
	}
	return defaultCodec.Unmarshal(data, v)
}

func useGogo(t reflect.Type) bool {
	if t == nil {
		return false
	}
	pkg := t.PkgPath()
	if strings.HasPrefix(pkg, jaegerProtoGenPkgPath) {
		return true
	}
	if strings.HasPrefix(pkg, jaegerModelPkgPath) {
		return true
	}
	return false
}
