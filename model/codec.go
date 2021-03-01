package model

import (
	proto "github.com/gogo/protobuf/proto"
	"google.golang.org/grpc/encoding"
)

func init() {
	encoding.RegisterCodec(newCodec())
}

// gogoCodec forces the use of gogo proto marshalling/unmarshalling for
// proto over grpc
type gogoCodec struct {
}

func newCodec() *gogoCodec {
	return &gogoCodec{}
}

// Name implements encoding.Codec
func (c *gogoCodec) Name() string {
	return "proto"
}

// Marshal implements encoding.Codec
func (c *gogoCodec) Marshal(v interface{}) ([]byte, error) {
	return proto.Marshal(v.(proto.Message))
}

// Unmarshal implements encoding.Codec
func (c *gogoCodec) Unmarshal(data []byte, v interface{}) error {
	return proto.Unmarshal(data, v.(proto.Message))
}
