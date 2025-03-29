package test

import (
	"testing"

	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"google.golang.org/protobuf/proto"
)

func TestTracesChunk(t *testing.T) {
	traceA := ptrace.NewTraces()
	spanA := traceA.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	spanA.SetName("span-a")

	c := new(TracesChunk)
	c.Traces = []*jptrace.TracesData{
		(*jptrace.TracesData)(&traceA),
	}
	buf, err := proto.Marshal(c)
	require.NoError(t, err)
	var c2 TracesChunk
	err = proto.Unmarshal(buf, &c2)
	require.NoError(t, err)
}
