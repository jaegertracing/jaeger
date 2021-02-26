package api_v2

import (
	"github.com/gogo/protobuf/codec"
	"google.golang.org/grpc/encoding"
)

type stupidcodec struct {
	codec.Codec
}

func (s *stupidcodec) Name() string {
	return s.String()
}

func NewStupidCodec(n int) *stupidcodec {
	return &stupidcodec{
		Codec: codec.New(n),
	}
}

func init() {
	encoding.RegisterCodec(NewStupidCodec(0))
}
