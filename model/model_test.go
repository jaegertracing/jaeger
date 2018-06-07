// Copyright (c) 2018 The Jaeger Authors.
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

package model

import (
	"testing"
	"time"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/stretchr/testify/require"
)

/*
TODO:
	- [done] Test if PB model can be used as domain model
	- Try a load balancer
	- Implement UNS-based resolver/balancer
	- Combine gRPC and HTTP handlers on the same port
	  https://github.com/philips/grpc-gateway-example/blob/master/cmd/serve.go
	  https://coreos.com/blog/gRPC-protobufs-swagger.html
	- Serve Swagger page
		- Swagger definition does not respect JSON serialization format, e.g. for TraceID

Decisions:
	- Naming TraceID/SpanID fields in proto, model, and JSON
		- currently going with TraceID for model, traceID for proto/JSON.
	- Do we want to keep using uint64s for IDs or bytes like Census
		- integers avoid memory allocations in Go for each ID
		- Census uses bytes: https://github.com/census-instrumentation/opencensus-proto/pull/45/files
		- decided to use uint64s
	- Do we want to keep or drop parentSpanID?
		- decided to exclude, too much logical complexity to interpret both parent ID and references
	- Are we ok with string rendering of duration, e.g. "1s", "15ms"?
		- proposal to render as floating point value in microseconds
		- currently using this default rendering
		- TODO: try float number
	- Do we want to use Map for tags?
		- decided to use list. Had a bunch of issues with ProcessMap in the actual map format.
	- Do we want to use oneof for different types of values in KeyValue?
		- oneof in Go uses polymorphism, which requires allocation for the wrapper object
		- decided to use flat list of optional fields and a Type field
	- Do we want to use oneof for Process (embedded) / ProcessID (reference)?
		- oneof in Go uses polymorphism, which requires allocation for the wrapper object
		- decided to use flat list of optional fields
*/

func TestProto(t *testing.T) {
	span := &Span{
		TraceID:   TraceID{High: 321, Low: 123},
		SpanID:    SpanID(456),
		StartTime: time.Now(),
		Duration:  42 * time.Microsecond,
		References: []SpanRef{
			{
				TraceID: TraceID{Low: 123},
				SpanID:  SpanID(456),
				RefType: SpanRefType_CHILD_OF,
			},
		},
		Tags: []KeyValue{
			{
				Key:   "baz",
				VType: ValueType_STRING,
				VStr:  "foo bar",
			},
		},
		Logs: []Log{
			{
				Timestamp: time.Now(),
				Fields: []KeyValue{
					{
						Key:   "baz",
						VType: ValueType_STRING,
						VStr:  "foo bar",
					},
				},
			},
		},
		// ProcessOrID: &Span_ProcessID{ProcessID: "p1"},
		// ProcessOrID: &Span_Process{Process: &Process{ServiceName: "foo"}},
	}
	marshaler := &jsonpb.Marshaler{}
	str, err := marshaler.MarshalToString(span)
	require.NoError(t, err)
	t.Log(str)

	kv1 := KeyValue{
		Key:   "baz",
		VType: ValueType_STRING,
		VStr:  "foo bar",
	}
	kv1b, err := kv1.Marshal()
	require.NoError(t, err)
	t.Logf("kv1 size=%d, bytes=%v", len(kv1b), kv1b)
}
