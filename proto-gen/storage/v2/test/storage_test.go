package test

import (
	"testing"

	v1 "go.opentelemetry.io/proto/otlp/trace/v1"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"google.golang.org/protobuf/proto"
)

func TestTracesChunk(t *testing.T) {
	traceA := ptrace.NewTraces()
	spanA := traceA.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	spanA.SetName("span-a")

	c := new(TracesChunk)
	c.Traces = []*v1.TracesData{
		{
			ResourceSpans: []*v1.ResourceSpans{
				{
					ScopeSpans: []*v1.ScopeSpans{
						{
							Spans: []*v1.Span{
								{
									Name: "span-a",
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
