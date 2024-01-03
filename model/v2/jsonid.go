// Copyright (c) 2023 The Jaeger Authors.
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package model

import (
	"encoding/base64"
	"fmt"
)

// marshalJSON converts trace id into a base64 string enclosed in quotes.
// Called by Protobuf JSON deserialization.
func marshalJSON(id []byte) ([]byte, error) {
	// Plus 2 quote chars at the start and end.
	hexLen := base64.StdEncoding.EncodedLen(len(id)) + 2

	b := make([]byte, hexLen)
	base64.StdEncoding.Encode(b[1:hexLen-1], id)
	b[0], b[hexLen-1] = '"', '"'

	return b, nil
}

// unmarshalJSON inflates trace id from base64 string, possibly enclosed in quotes.
// Called by Protobuf JSON deserialization.
func UnmarshalJSON(dst []byte, src []byte) error {
	if l := len(src); l >= 2 && src[0] == '"' && src[l-1] == '"' {
		src = src[1 : l-1]
	}
	nLen := len(src)
	if nLen == 0 {
		return nil
	}

	if l := base64.StdEncoding.DecodedLen(nLen); len(dst) != l {
		return fmt.Errorf("invalid length for ID '%s', dst=%d, src=%d", string(src), len(dst), l)
	}

	_, err := base64.StdEncoding.Decode(dst, src)
	if err != nil {
		return fmt.Errorf("cannot unmarshal ID from string '%s': %w", string(src), err)
	}
	return nil
}
