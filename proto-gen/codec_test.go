package proto_gen

import (
	"github.com/golang/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/model"
)

func TestCodecMarshallAndUnmarshall(t *testing.T) {
	c := newCodec()
	s1 := &model.Span{OperationName: "foo", TraceID: model.NewTraceID(1, 2)}
	data, err := c.Marshal(s1)
	require.NoError(t, err)

	s2 := &model.Span{}
	err = c.Unmarshal(data, s2)
	require.NoError(t, err)
	assert.Equal(t, s1, s2)
}

func TestWireCompatibility(t *testing.T) {
	c := newCodec()
	s1 := &model.Span{OperationName: "foo", TraceID: model.NewTraceID(1, 2)}
	data, err := c.Marshal(s1)
	require.NoError(t, err)

	var goprotoMessage emptypb.Empty
	err = proto.Unmarshal(data, &goprotoMessage)
	require.NoError(t, err)

	data2, err := proto.Marshal(&goprotoMessage)
	require.NoError(t, err)

	s2 := &model.Span{}
	err = c.Unmarshal(data2, s2)
	require.NoError(t, err)
	assert.Equal(t, s1, s2)
}
