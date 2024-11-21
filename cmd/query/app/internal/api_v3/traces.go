// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package api_v3

import (
	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/pkg/gogocodec"
)

// TracesData is an alias to ptrace.Traces that supports Gogo marshaling.
// Our .proto APIs may refer to otlp.TraceData type, but its corresponding
// protoc-generated struct is internal in OTel Collector, so we substitute
// it for this TracesData type that implements marshaling methods by
// delegating to public functions in the OTel Collector's ptrace module.
type TracesData ptrace.Traces

var (
	_ gogocodec.CustomType = (*TracesData)(nil)
	_ proto.Message        = (*TracesData)(nil)
)

func (td TracesData) ToTraces() ptrace.Traces {
	return ptrace.Traces(td)
}

// Marshal implements gogocodec.CustomType.
func (td *TracesData) Marshal() ([]byte, error) {
	return new(ptrace.ProtoMarshaler).MarshalTraces(td.ToTraces())
}

// MarshalTo implements gogocodec.CustomType.
func (*TracesData) MarshalTo([]byte /* data */) (n int, err error) {
	// TODO unclear when this might be called, perhaps when type is embedded inside other structs.
	panic("unimplemented")
}

// MarshalToSizedBuffer is used by Gogo.
func (td *TracesData) MarshalToSizedBuffer(buf []byte) (int, error) {
	data, err := td.Marshal()
	if err != nil {
		return 0, err
	}
	copy(buf, data)
	return len(data), nil
}

// MarshalJSONPB implements gogocodec.CustomType.
func (td *TracesData) MarshalJSONPB(*jsonpb.Marshaler) ([]byte, error) {
	return new(ptrace.JSONMarshaler).MarshalTraces(td.ToTraces())
}

// UnmarshalJSONPB implements gogocodec.CustomType.
func (td *TracesData) UnmarshalJSONPB(_ *jsonpb.Unmarshaler, data []byte) error {
	t, err := new(ptrace.JSONUnmarshaler).UnmarshalTraces(data)
	if err != nil {
		return err
	}
	*td = TracesData(t)
	return nil
}

// Size implements gogocodec.CustomType.
func (td *TracesData) Size() int {
	return new(ptrace.ProtoMarshaler).TracesSize(td.ToTraces())
}

// Unmarshal implements gogocodec.CustomType.
func (td *TracesData) Unmarshal(data []byte) error {
	t, err := new(ptrace.ProtoUnmarshaler).UnmarshalTraces(data)
	if err != nil {
		return err
	}
	*td = TracesData(t)
	return nil
}

// ProtoMessage implements proto.Message.
func (*TracesData) ProtoMessage() {
	// nothing to do here
}

// Reset implements proto.Message.
func (td *TracesData) Reset() {
	*td = TracesData(ptrace.NewTraces())
}

// String implements proto.Message.
func (*TracesData) String() string {
	return "*TracesData"
}
