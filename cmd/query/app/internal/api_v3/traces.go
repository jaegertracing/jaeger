package api_v3

import (
	"github.com/gogo/protobuf/jsonpb"
	proto "github.com/gogo/protobuf/proto"
	"github.com/jaegertracing/jaeger/pkg/gogocodec"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

type TracesData ptrace.Traces

var _ gogocodec.CustomType = (*TracesData)(nil)
var _ proto.Message = (*TracesData)(nil)

func (td TracesData) ToTraces() ptrace.Traces {
	return ptrace.Traces(td)
}

// Marshal implements gogocodec.CustomType.
func (td *TracesData) Marshal() ([]byte, error) {
	return new(ptrace.ProtoMarshaler).MarshalTraces(td.ToTraces())
}

// MarshalTo implements gogocodec.CustomType.
func (*TracesData) MarshalTo(data []byte) (n int, err error) {
	// unclear when this might be called, perhaps when type is embedded inside other structs.
	panic("unimplemented")
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
	*td = TracesData{}
}

// String implements proto.Message.
func (*TracesData) String() string {
	return "*TracesData"
}
