// Copyright (c) 2023 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package json

import (
	"bytes"
	"strings"

	"github.com/gogo/protobuf/jsonpb"

	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
)

// SamplingStrategyResponseToJSON defines the official way to generate
// a JSON response from /sampling endpoints.
func SamplingStrategyResponseToJSON(protoObj *api_v2.SamplingStrategyResponse) (string, error) {
	// For backwards compatibility with Thrift-to-JSON encoding,
	// we want the output to include "strategyType":"PROBABILISTIC" when appropriate.
	// However, due to design oversight, the enum value for PROBABILISTIC is 0, so
	// we need to set EmitDefaults=true. This in turns causes null fields to be emitted too,
	// so we take care of them below.
	jsonpbMarshaler := jsonpb.Marshaler{
		EmitDefaults: true,
	}

	str, err := jsonpbMarshaler.MarshalToString(protoObj)
	if err != nil {
		return "", err
	}

	// Because we set EmitDefaults, jsonpb will also render null entries, so we remove them here.
	str = strings.ReplaceAll(str, `"probabilisticSampling":null,`, "")
	str = strings.ReplaceAll(str, `,"rateLimitingSampling":null`, "")
	str = strings.ReplaceAll(str, `,"operationSampling":null`, "")

	return str, nil
}

// SamplingStrategyResponseFromJSON is the official way to parse strategy in JSON.
func SamplingStrategyResponseFromJSON(json []byte) (*api_v2.SamplingStrategyResponse, error) {
	var obj api_v2.SamplingStrategyResponse
	if err := jsonpb.Unmarshal(bytes.NewReader(json), &obj); err != nil {
		return nil, err
	}
	return &obj, nil
}
