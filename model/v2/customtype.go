// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package model

import "github.com/gogo/protobuf/proto"

// gogoCustom is an interface that Gogo expects custom types to implement.
// https://github.com/gogo/protobuf/blob/master/custom_types.md
type gogoCustom interface {
	Marshal() ([]byte, error)
	MarshalTo(data []byte) (n int, err error)
	Unmarshal(data []byte) error

	proto.Sizer

	MarshalJSON() ([]byte, error)
	UnmarshalJSON(data []byte) error
}
