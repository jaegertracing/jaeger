package test

import (
	"testing"

	v1 "go.opentelemetry.io/proto/otlp/trace/v1"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestTracesChunk(t *testing.T) {
	traceId := [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	spanId := [8]byte{1, 2, 3, 4, 5, 6, 7, 8}
	t.Logf("span-name: %x", ([]byte("span-a")))

	c := new(TracesChunk)
	c.Traces = []*v1.TracesData{
		{
			ResourceSpans: []*v1.ResourceSpans{
				{
					ScopeSpans: []*v1.ScopeSpans{
						{
							Spans: []*v1.Span{
								{
									Name:    "span-a",
									TraceId: traceId[:],
									SpanId:  spanId[:],
								},
							},
						},
					},
				},
			},
		},
	}
	buf, err := proto.Marshal(c)
	t.Logf("buf: %x", buf)
	require.NoError(t, err)
	var c2 TracesChunk
	err = proto.Unmarshal(buf, &c2)
	require.NoError(t, err)
	require.Equal(t, c, &c2)
}
