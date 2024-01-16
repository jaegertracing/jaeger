// Copyright (c) 2021 The Jaeger Authors.
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

package gogocodec

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/testutils"
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

func TestUseGogo(t *testing.T) {
	assert.False(t, useGogo(nil))

	var span model.Span
	assert.True(t, useGogo(reflect.TypeOf(span)))
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
