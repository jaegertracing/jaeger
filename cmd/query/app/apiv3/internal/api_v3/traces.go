package api_v3

import (
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
	panic("unimplemented")
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

// MarshalJSON implements gogocodec.CustomType.
func (*TracesData) MarshalJSON() ([]byte, error) {
	panic("unimplemented")
}

// UnmarshalJSON implements gogocodec.CustomType.
func (*TracesData) UnmarshalJSON(data []byte) error {
	panic("unimplemented")
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
