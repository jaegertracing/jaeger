package proto_gen

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/jaegertracing/jaeger/model"
)

func TestCodecMarshallAndUnmarshall_jaeger_type(t *testing.T) {
	c := newCodec()
	s1 := &model.Span{OperationName: "foo", TraceID: model.NewTraceID(1, 2)}
	data, err := c.Marshal(s1)
	require.NoError(t, err)

	s2 := &model.Span{}
	err = c.Unmarshal(data, s2)
	require.NoError(t, err)
	assert.Equal(t, s1, s2)
}

func TestCodecMarshallAndUnmarshall_no_jaeger_type(t *testing.T) {
	c := newCodec()
	goprotoMessage1 := &emptypb.Empty{}
	data, err := c.Marshal(goprotoMessage1)
	require.NoError(t, err)

	goprotoMessage2 := &emptypb.Empty{}
	err = c.Unmarshal(data, goprotoMessage2)
	require.NoError(t, err)
	assert.Equal(t, goprotoMessage1, goprotoMessage2)
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
