package test

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "go.opentelemetry.io/proto/otlp/trace/v1"
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
	require.NoError(t, err)
	t.Logf("buf: %x", buf)

	thisTest := "0a2a0a28122612240a100102030405060708090a0b0c0d0e0f10120801020304050607082a067370616e2d61"
	assert.Equal(t, thisTest, hex.EncodeToString(buf))

	var c2 TracesChunk
	err = proto.Unmarshal(buf, &c2)
	require.NoError(t, err)
	assert.Equal(t, c, &c2)

	otherTest := "0a320a00122c0a0012280a100102030405060708090a0b0c0d0e0f101208010203040506070822002a067370616e2d617a000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"
	buf, err = hex.DecodeString(otherTest)
	require.NoError(t, err)
	err = proto.Unmarshal(buf, &c2)
	require.NoError(t, err)
	assert.Equal(t, c, &c2)
}
