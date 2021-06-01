// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package v1

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
)

// JSONPb is a copy of https://github.com/grpc-ecosystem/grpc-gateway/blob/master/runtime/marshal_jsonpb.go
// with one difference: github.com/golang/protobuf imports are replaced by github.com/gogo/protobuf
// to make it work with Gogoproto messages that we use. There are no other changes to
// JSONPb done. It should be safe to update (copy again) it to latest version of
// https://github.com/grpc-ecosystem/grpc-gateway/blob/master/runtime/marshal_jsonpb.go
// when the github.com/grpc-ecosystem/grpc-gateway dependency is updated.

//lint:file-ignore S1034 Ignore lint errors, this is a copied file and we don't want to modify it.

// JSONPb is a Marshaler which marshals/unmarshals into/from JSON
// with the "github.com/golang/protobuf/jsonpb".
// It supports fully functionality of protobuf unlike JSONBuiltin.
//
// The NewDecoder method returns a DecoderWrapper, so the underlying
// *json.Decoder methods can be used.
type JSONPb jsonpb.Marshaler

// ContentType always returns "application/json".
func (*JSONPb) ContentType() string {
	return "application/json"
}

// Marshal marshals "v" into JSON.
func (j *JSONPb) Marshal(v interface{}) ([]byte, error) {
	if _, ok := v.(proto.Message); !ok {
		return j.marshalNonProtoField(v)
	}

	var buf bytes.Buffer
	if err := j.marshalTo(&buf, v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (j *JSONPb) marshalTo(w io.Writer, v interface{}) error {
	p, ok := v.(proto.Message)
	if !ok {
		buf, err := j.marshalNonProtoField(v)
		if err != nil {
			return err
		}
		_, err = w.Write(buf)
		return err
	}
	return (*jsonpb.Marshaler)(j).Marshal(w, p)
}

var (
	// protoMessageType is stored to prevent constant lookup of the same type at runtime.
	protoMessageType = reflect.TypeOf((*proto.Message)(nil)).Elem()
)

// marshalNonProto marshals a non-message field of a protobuf message.
// This function does not correctly marshals arbitrary data structure into JSON,
// but it is only capable of marshaling non-message field values of protobuf,
// i.e. primitive types, enums; pointers to primitives or enums; maps from
// integer/string types to primitives/enums/pointers to messages.
func (j *JSONPb) marshalNonProtoField(v interface{}) ([]byte, error) {
	if v == nil {
		return []byte("null"), nil
	}
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return []byte("null"), nil
		}
		rv = rv.Elem()
	}

	if rv.Kind() == reflect.Slice {
		if rv.IsNil() {
			if j.EmitDefaults {
				return []byte("[]"), nil
			}
			return []byte("null"), nil
		}

		if rv.Type().Elem().Implements(protoMessageType) {
			var buf bytes.Buffer
			err := buf.WriteByte('[')
			if err != nil {
				return nil, err
			}
			for i := 0; i < rv.Len(); i++ {
				if i != 0 {
					err = buf.WriteByte(',')
					if err != nil {
						return nil, err
					}
				}
				if err = (*jsonpb.Marshaler)(j).Marshal(&buf, rv.Index(i).Interface().(proto.Message)); err != nil {
					return nil, err
				}
			}
			err = buf.WriteByte(']')
			if err != nil {
				return nil, err
			}

			return buf.Bytes(), nil
		}
	}

	if rv.Kind() == reflect.Map {
		m := make(map[string]*json.RawMessage)
		for _, k := range rv.MapKeys() {
			buf, err := j.Marshal(rv.MapIndex(k).Interface())
			if err != nil {
				return nil, err
			}
			m[fmt.Sprintf("%v", k.Interface())] = (*json.RawMessage)(&buf)
		}
		if j.Indent != "" {
			return json.MarshalIndent(m, "", j.Indent)
		}
		return json.Marshal(m)
	}
	if enum, ok := rv.Interface().(protoEnum); ok && !j.EnumsAsInts {
		return json.Marshal(enum.String())
	}
	return json.Marshal(rv.Interface())
}

// Unmarshal unmarshals JSON "data" into "v"
func (j *JSONPb) Unmarshal(data []byte, v interface{}) error {
	return unmarshalJSONPb(data, v)
}

// NewDecoder returns a Decoder which reads JSON stream from "r".
func (j *JSONPb) NewDecoder(r io.Reader) runtime.Decoder {
	d := json.NewDecoder(r)
	return DecoderWrapper{Decoder: d}
}

// DecoderWrapper is a wrapper around a *json.Decoder that adds
// support for protos to the Decode method.
type DecoderWrapper struct {
	*json.Decoder
}

// Decode wraps the embedded decoder's Decode method to support
// protos using a jsonpb.Unmarshaler.
func (d DecoderWrapper) Decode(v interface{}) error {
	return decodeJSONPb(d.Decoder, v)
}

// NewEncoder returns an Encoder which writes JSON stream into "w".
func (j *JSONPb) NewEncoder(w io.Writer) runtime.Encoder {
	return runtime.EncoderFunc(func(v interface{}) error {
		if err := j.marshalTo(w, v); err != nil {
			return err
		}
		// mimic json.Encoder by adding a newline (makes output
		// easier to read when it contains multiple encoded items)
		_, err := w.Write(j.Delimiter())
		return err
	})
}

func unmarshalJSONPb(data []byte, v interface{}) error {
	d := json.NewDecoder(bytes.NewReader(data))
	return decodeJSONPb(d, v)
}

func decodeJSONPb(d *json.Decoder, v interface{}) error {
	p, ok := v.(proto.Message)
	if !ok {
		return decodeNonProtoField(d, v)
	}
	unmarshaler := &jsonpb.Unmarshaler{AllowUnknownFields: allowUnknownFields}
	return unmarshaler.UnmarshalNext(d, p)
}

func decodeNonProtoField(d *json.Decoder, v interface{}) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr {
		return fmt.Errorf("%T is not a pointer", v)
	}
	for rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			rv.Set(reflect.New(rv.Type().Elem()))
		}
		if rv.Type().ConvertibleTo(typeProtoMessage) {
			unmarshaler := &jsonpb.Unmarshaler{AllowUnknownFields: allowUnknownFields}
			return unmarshaler.UnmarshalNext(d, rv.Interface().(proto.Message))
		}
		rv = rv.Elem()
	}
	if rv.Kind() == reflect.Map {
		if rv.IsNil() {
			rv.Set(reflect.MakeMap(rv.Type()))
		}
		conv, ok := convFromType[rv.Type().Key().Kind()]
		if !ok {
			return fmt.Errorf("unsupported type of map field key: %v", rv.Type().Key())
		}

		m := make(map[string]*json.RawMessage)
		if err := d.Decode(&m); err != nil {
			return err
		}
		for k, v := range m {
			result := conv.Call([]reflect.Value{reflect.ValueOf(k)})
			if err := result[1].Interface(); err != nil {
				return err.(error)
			}
			bk := result[0]
			bv := reflect.New(rv.Type().Elem())
			if err := unmarshalJSONPb([]byte(*v), bv.Interface()); err != nil {
				return err
			}
			rv.SetMapIndex(bk, bv.Elem())
		}
		return nil
	}
	if _, ok := rv.Interface().(protoEnum); ok {
		var repr interface{}
		if err := d.Decode(&repr); err != nil {
			return err
		}
		switch repr.(type) {
		case string:
			// TODO(yugui) Should use proto.StructProperties?
			return fmt.Errorf("unmarshaling of symbolic enum %q not supported: %T", repr, rv.Interface())
		case float64:
			rv.Set(reflect.ValueOf(int32(repr.(float64))).Convert(rv.Type()))
			return nil
		default:
			return fmt.Errorf("cannot assign %#v into Go type %T", repr, rv.Interface())
		}
	}
	return d.Decode(v)
}

type protoEnum interface {
	fmt.Stringer
	EnumDescriptor() ([]byte, []int)
}

var typeProtoMessage = reflect.TypeOf((*proto.Message)(nil)).Elem()

// Delimiter for newline encoded JSON streams.
func (j *JSONPb) Delimiter() []byte {
	return []byte("\n")
}

// allowUnknownFields helps not to return an error when the destination
// is a struct and the input contains object keys which do not match any
// non-ignored, exported fields in the destination.
var allowUnknownFields = true

// DisallowUnknownFields enables option in decoder (unmarshaler) to
// return an error when it finds an unknown field. This function must be
// called before using the JSON marshaler.
func DisallowUnknownFields() {
	allowUnknownFields = false
}

// convFromType is an exact copy from https://github.com/grpc-ecosystem/grpc-gateway/blob/master/runtime/query.go
var (
	convFromType = map[reflect.Kind]reflect.Value{
		reflect.String:  reflect.ValueOf(runtime.String),
		reflect.Bool:    reflect.ValueOf(runtime.Bool),
		reflect.Float64: reflect.ValueOf(runtime.Float64),
		reflect.Float32: reflect.ValueOf(runtime.Float32),
		reflect.Int64:   reflect.ValueOf(runtime.Int64),
		reflect.Int32:   reflect.ValueOf(runtime.Int32),
		reflect.Uint64:  reflect.ValueOf(runtime.Uint64),
		reflect.Uint32:  reflect.ValueOf(runtime.Uint32),
		reflect.Slice:   reflect.ValueOf(runtime.Bytes),
	}
)
