package adjuster

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/uber/jaeger/model"
)

func TestClockSkewAdjuster(t *testing.T) {
	// spanProto is a simple descriptor of complete model.Span
	type spanProto struct {
		id, parent, startTime, duration int
		logs                            []int // timestamps for logs
		host                            string
		adjusted                        int   // start time after adjustment
		adjustedLogs                    []int // adjusted log timestamps
	}

	// helper function that constructs a trace from a list of span prototypes
	makeTrace := func(spanPrototypes []spanProto) *model.Trace {
		trace := &model.Trace{}
		for _, spanProto := range spanPrototypes {
			var logs []model.Log
			for _, log := range spanProto.logs {
				logs = append(logs, model.Log{
					Timestamp: uint64(log),
					Fields:    []model.KeyValue{model.String("event", "some event")},
				})
			}
			span := &model.Span{
				TraceID:      model.TraceID{Low: 1},
				SpanID:       model.SpanID(spanProto.id),
				ParentSpanID: model.SpanID(spanProto.parent),
				StartTime:    uint64(spanProto.startTime),
				Duration:     uint64(spanProto.duration),
				Logs:         logs,
				Process: &model.Process{
					ServiceName: spanProto.host,
					Tags: []model.KeyValue{
						model.String("ip", spanProto.host),
					},
				},
			}
			trace.Spans = append(trace.Spans, span)
		}
		return trace
	}

	testCases := []struct {
		description string
		trace       []spanProto
		err         string
	}{
		{
			description: "single span with bad parent",
			trace: []spanProto{
				{id: 1, parent: 99, startTime: 0, duration: 100, host: "a", adjusted: 0},
			},
			err: "invalid parent span ID=63 in the span with ID=1", // 99 == 0x63
		},
		{
			description: "two span with the same ID",
			trace: []spanProto{
				{id: 1, parent: 0, startTime: 0, duration: 100, host: "a", adjusted: 0},
				{id: 1, parent: 0, startTime: 0, duration: 100, host: "a", adjusted: 0},
			},
			err: "found more than one span with ID=1",
		},
		{
			description: "parent-child on the same host",
			trace: []spanProto{
				{id: 1, parent: 0, startTime: 0, duration: 100, host: "a", adjusted: 0},
				{id: 2, parent: 1, startTime: 10, duration: 50, host: "a", adjusted: 10},
			},
		},
		{
			description: "do not ajust parent-child on the same host",
			trace: []spanProto{
				{id: 1, parent: 0, startTime: 10, duration: 100, host: "a", adjusted: 10},
				{id: 2, parent: 1, startTime: 0, duration: 50, host: "a", adjusted: 0},
			},
		},
		{
			description: "do not adjust child that fits inside parent",
			trace: []spanProto{
				{id: 1, parent: 0, startTime: 10, duration: 100, host: "a", adjusted: 10},
				{id: 2, parent: 1, startTime: 20, duration: 50, host: "b", adjusted: 20},
			},
		},
		{
			description: "do not ajust child that is longer than parent",
			trace: []spanProto{
				{id: 1, parent: 0, startTime: 10, duration: 100, host: "a", adjusted: 10},
				{id: 2, parent: 1, startTime: 20, duration: 150, host: "b", adjusted: 20},
			},
		},
		{
			description: "adjust child starting before parent",
			trace: []spanProto{
				{id: 1, parent: 0, startTime: 10, duration: 100, host: "a", adjusted: 10},
				// latency = (100-50) / 2 = 25
				// delta = (10 - 0) + latency = 35
				{id: 2, parent: 1, startTime: 0, duration: 50, host: "b", adjusted: 35,
					logs: []int{5, 10}, adjustedLogs: []int{40, 45}},
			},
		},
		{
			description: "adjust child starting before parent even if it is longer",
			trace: []spanProto{
				{id: 1, parent: 0, startTime: 10, duration: 100, host: "a", adjusted: 10},
				{id: 2, parent: 1, startTime: 0, duration: 150, host: "b", adjusted: 10},
			},
		},
		{
			description: "adjust child ending after parent but being shorter",
			trace: []spanProto{
				{id: 1, parent: 0, startTime: 10, duration: 100, host: "a", adjusted: 10},
				// latency: (100 - 70) / 2 = 15
				// new child start time: 10 + latency = 25, delta = -25
				{id: 2, parent: 1, startTime: 50, duration: 70, host: "b", adjusted: 25},
				// same host 'b', so same delta = -25
				// new start time: 60 + delta = 35
				{id: 3, parent: 2, startTime: 60, duration: 20, host: "b", adjusted: 35,
					logs: []int{65, 70}, adjustedLogs: []int{40, 45}},
			},
		},
	}

	for _, tt := range testCases {
		testCase := tt // capture loop var
		t.Run(testCase.description, func(t *testing.T) {
			adjuster := ClockSkew()
			trace, err := adjuster.Adjust(makeTrace(testCase.trace))
			if testCase.err != "" {
				assert.EqualError(t, err, testCase.err)
			} else {
				assert.NoError(t, err)
			}
			for _, proto := range testCase.trace {
				id := proto.id
				span := trace.FindSpanByID(model.SpanID(uint64(id)))
				require.NotNil(t, span, "expecting span with span ID = %d", id)
				// compare values as int because assert.Equal prints uint64 as hex
				assert.Equal(
					t, int(proto.adjusted), int(span.StartTime),
					"adjusted start time of span[ID = %d]", id)
				for i, logTs := range proto.adjustedLogs {
					assert.Equal(
						t, int(logTs), int(span.Logs[i].Timestamp),
						"adjusted log timestamp of span[ID = %d], log[%d]", id, i)
				}
			}
		})
	}
}

func TestHostKey(t *testing.T) {
	testCases := []struct {
		tag     model.KeyValue
		hostKey string
	}{
		{tag: model.String("ip", "1.2.3.4"), hostKey: "1.2.3.4"},
		{tag: model.String("ipv4", "1.2.3.4"), hostKey: ""},
		{tag: model.Int64("ip", int64(1<<24|2<<16|3<<8|4)), hostKey: "1.2.3.4"},
		{tag: model.Binary("ip", []byte{1, 2, 3, 4}), hostKey: "1.2.3.4"},
		{tag: model.Binary("ip", []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff, 1, 2, 3, 4}), hostKey: "1.2.3.4"},
		{tag: model.Binary("ip", []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 2, 3, 4}), hostKey: "::102:304"},
		{tag: model.Binary("ip", []byte{1, 2, 3, 4, 5}), hostKey: ""},
		{tag: model.Float64("ip", 123.4), hostKey: ""},
	}

	for _, tt := range testCases {
		testCase := tt // capture loop var
		t.Run(fmt.Sprintf("%+v", testCase.tag), func(t *testing.T) {
			span := &model.Span{
				Process: &model.Process{
					ServiceName: "some service",
					Tags:        []model.KeyValue{testCase.tag},
				},
			}
			hostKey := hostKey(span)
			assert.Equal(t, testCase.hostKey, hostKey)
		})
	}
}
