package model_test

import (
	"testing"

	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/model"
	model2 "github.com/jaegertracing/jaeger/proto-gen/model"
)

func TestSpanToProto(t *testing.T) {
	span1 := &model.Span{
		OperationName: "x",
	}
	span2 := &model2.Span{
		OperationName: "x",
	}

	s1, err := proto.Marshal(span1)
	require.NoError(t, err)

	s2, err := proto.Marshal(span2)
	require.NoError(t, err)

	require.Equal(t, s1, s2)
}
