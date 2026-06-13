// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"
)

type jsonMarshaler = func(writer io.Writer, response any) error

// newProtoJSONMarshaler returns a protobuf-friendly JSON marshaler that knows how to handle protobuf-specific
// field types such as "oneof" as well as dealing with NaNs which are not supported by JSON.
func newProtoJSONMarshaler(prettyPrint bool) jsonMarshaler {
	marshaler := new(jsonpb.Marshaler)
	if prettyPrint {
		marshaler.Indent = prettyPrintIndent
	}
	return func(w io.Writer, response any) error {
		return marshaler.Marshal(w, response.(proto.Message))
	}
}

// newStructJSONMarshaler returns a marshaler that uses the built-in encoding/json package for marshaling basic structs to JSON.
func newStructJSONMarshaler(prettyPrint bool) jsonMarshaler {
	marshaler := json.Marshal
	if prettyPrint {
		marshaler = func(v any) ([]byte, error) {
			return json.MarshalIndent(v, "", prettyPrintIndent)
		}
	}
	return func(w io.Writer, response any) error {
		resp, err := marshaler(response)
		if err != nil {
			return fmt.Errorf("failed marshalling HTTP response to JSON: %w", err)
		}
		_, err = w.Write(resp)
		return err
	}
}
