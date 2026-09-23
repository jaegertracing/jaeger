// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jptrace

import (
	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/gogocodec"
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
func (td *TracesData) MarshalTo(buf []byte) (n int, err error) {
	return td.MarshalToSizedBuffer(buf)
}

// MarshalToSizedBuffer writes the encoded traces into the tail of buf and returns the number of
// bytes written. That is the contract gogo-generated code relies on: a parent message encodes
// its fields back to front, hands each nested field the still-unused prefix of its buffer, and
// takes the field's length from the return value. Writing at the head of buf instead would leave
// the slot the parent reserved at the tail empty, and the parent would then encode its remaining
// fields, including this field's own tag and length, over what we would have written.
func (td *TracesData) MarshalToSizedBuffer(buf []byte) (int, error) {
	data, err := td.Marshal()
	if err != nil {
		return 0, err
	}
	n := copy(buf[len(buf)-len(data):], data)
	return n, nil
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
