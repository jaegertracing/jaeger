package protomodel

import (
	"testing"
	"time"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/stretchr/testify/require"
)

/*
TODO:
	- Test if PB model can be used as domain model
	- Try a load balancer
	- Implement UNS-based resolver/balancer
	- May still need to go to `bytes` encoding for trace/spanID
	- Combine gRPC and HTTP handlers on the same port
	  https://github.com/philips/grpc-gateway-example/blob/master/cmd/serve.go
	  https://coreos.com/blog/gRPC-protobufs-swagger.html
	- Serve Swagger page
		- Swagger definition does not respect JSON serialization format, e.g. for TraceID

Decisions:
	- Naming TraceID/SpanID fields in proto, model, and JSON
		- currently going with TraceID for model, traceID for proto/JSON.
	- TraceID and SpanID need custom JSON marshaler to be rendered as strings
		- MarshalJSON done
		- TODO UnmarshalJSON
	- Do we want to keep using uint64s for IDs or bytes like Census
		- integers avoid memory allocations in Go for each ID
		- Census uses bytes: https://github.com/census-instrumentation/opencensus-proto/pull/45/files
		- currently using uint64s
	- Do we want to keep or drop parentSpanID?
		- currently excluded, too much logical complexity to interpret both parent ID and references
	- Are we ok with string rendering of duration, e.g. "1s", "15ms"?
		- proposal to render as floating point value in microseconds
		- currently using this default rendering
		- TODO try float number
	- Do we want to use Map for tags?
		- currently using list
	- Do we want to use oneof for different types of values in KeyValue?
		- oneof in Go uses polymorphism, which requires allocation for the wrapper object
		- currently using flat list of optional fields and a Type field
	- Do we want to use oneof for Process (embedded) / ProcessID (reference)?
		- oneof in Go uses polymorphism, which requires allocation for the wrapper object
		- undecided
*/

func TestProto(t *testing.T) {
	span := &Span{
		TraceID:   TraceID{High: 321, Low: 123},
		SpanID:    NewSpanID(456),
		StartTime: time.Now(),
		Duration:  42 * time.Microsecond,
		References: []SpanRef{
			SpanRef{
				TraceID: TraceID{Low: 123},
				SpanID:  NewSpanID(456),
				Type:    SpanRefType_CHILD_OF,
			},
		},
		Tags: []KeyValue{
			KeyValue{
				Key:   "baz",
				VType: ValueType_STRING,
				VStr:  "foo bar",
			},
		},
		Logs: []Log{
			Log{
				Timestamp: time.Now(),
				Fields: []KeyValue{
					KeyValue{
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

	kv2 := KeyValue2{
		Key:   "baz",
		Value: &KeyValue2_VStr{"foo bar"},
	}
	kv2b, err := kv2.Marshal()
	require.NoError(t, err)
	t.Logf("kv2 size=%d, bytes=%v", len(kv2b), kv2b)
}

// func BenchmarkSample(b *testing.B) {
// 	span := &Span{
// 		TraceID:   TraceID{High: 321, Low: 123},
// 		SpanID:    SpanID{456},
// 		StartTime: time.Now(),
// 	}
// 	for i := 0; i < b.N; i++ {
// 		// span.ProcessOrID = &Span_ProcessID{ProcessID: "foo bar"}
// 		span.ProcessOrID = &Span_Process{}
// 	}
// }
