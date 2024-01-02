// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package model

// gogoCustom is an interface that Gogo expects custom types to implement.
// https://github.com/gogo/protobuf/blob/master/proto/custom_gogo.go#L33
type gogoCustom interface {
	Marshal() ([]byte, error)
	Unmarshal(data []byte) error
	Size() int
}
