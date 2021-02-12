package app

import "github.com/gogo/protobuf/codec"

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
