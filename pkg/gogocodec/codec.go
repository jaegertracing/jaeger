// Copyright (c) 2021 The Jaeger Authors.
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

package gogocodec

import (
	"reflect"
	"strings"

	"github.com/gogo/protobuf/jsonpb"
	gogoproto "github.com/gogo/protobuf/proto"
	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/encoding/proto"
)

const (
	jaegerProtoGenPkgPath = "github.com/jaegertracing/jaeger/proto-gen"
	jaegerModelPkgPath    = "github.com/jaegertracing/jaeger/model"
)

var defaultCodec encoding.Codec

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
	defaultCodec = encoding.GetCodec(proto.Name)
	defaultCodec.Name() // ensure it's not nil
	encoding.RegisterCodec(newCodec())
}

// gogoCodec forces the use of gogo proto marshalling/unmarshalling for
// Jaeger proto types (package jaeger/gen-proto).
type gogoCodec struct{}

var _ encoding.Codec = (*gogoCodec)(nil)

func newCodec() *gogoCodec {
	return &gogoCodec{}
}

// Name implements encoding.Codec
func (*gogoCodec) Name() string {
	return proto.Name
}

// Marshal implements encoding.Codec
func (*gogoCodec) Marshal(v any) ([]byte, error) {
	t := reflect.TypeOf(v)
	elem := t.Elem()
	// use gogo proto only for Jaeger types
	if useGogo(elem) {
		return gogoproto.Marshal(v.(gogoproto.Message))
	}
	return defaultCodec.Marshal(v)
}

// Unmarshal implements encoding.Codec
func (*gogoCodec) Unmarshal(data []byte, v any) error {
	t := reflect.TypeOf(v)
	elem := t.Elem() // only for collections
	// use gogo proto only for Jaeger types
	if useGogo(elem) {
		return gogoproto.Unmarshal(data, v.(gogoproto.Message))
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
