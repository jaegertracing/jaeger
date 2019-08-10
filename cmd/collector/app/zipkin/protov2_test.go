// Copyright (c) 2019 The Jaeger Authors.
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

package zipkin

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	zipkinProto "github.com/jaegertracing/jaeger/proto-gen/zipkin"
	zmodel "github.com/jaegertracing/jaeger/proto-gen/zipkin"
	"github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
)

func TestProtoSpanFixtures(t *testing.T) {
	var spans zmodel.ListOfSpans
	loadJSON(t, "fixtures/zipkin_proto_01.json", &spans)
	fmt.Println(spans)
	tSpans, err := protoSpansV2ToThrift(&spans)
	require.NoError(t, err)
	assert.Equal(t, len(tSpans), 1)
	var pid int64 = 1
	var ts int64 = 1
	var d int64 = 10
	fmt.Println(tSpans)
	localE := &zipkincore.Endpoint{ServiceName: "foo", Ipv4: 170594602}
	remoteE := &zipkincore.Endpoint{ServiceName: "bar", Ipv4: 170594603}
	var highID = int64(4793352529331701374)
	fmt.Println(highID)
	tSpan := &zipkincore.Span{ID: 2, TraceID: int64(4795885597963667071), TraceIDHigh: &highID, ParentID: &pid, Name: "foo", Debug: true, Duration: &d, Timestamp: &ts,
		Annotations: []*zipkincore.Annotation{
			{Value: "foo", Timestamp: 1, Host: localE},
			{Value: zipkincore.CLIENT_SEND, Timestamp: ts, Host: localE},
			{Value: zipkincore.CLIENT_RECV, Timestamp: ts + d, Host: localE}},
		BinaryAnnotations: []*zipkincore.BinaryAnnotation{
			{Key: "foo", Value: []byte("bar"), Host: localE, AnnotationType: zipkincore.AnnotationType_STRING},
			{Key: zipkincore.SERVER_ADDR, Host: remoteE, AnnotationType: zipkincore.AnnotationType_BOOL}}}
	assert.Equal(t, tSpan, tSpans[0])
}

func TestLCFromProtoSpanLocalEndpoint(t *testing.T) {
	var spans zmodel.ListOfSpans
	loadProto(t, "fixtures/zipkin_proto_02.json", &spans)
	tSpans, err := protoSpansV2ToThrift(&spans)
	fmt.Println(tSpans)
	require.NoError(t, err)
	assert.Equal(t, len(tSpans), 1)
	var ts int64 = 1
	var d int64 = 10
	tSpan := &zipkincore.Span{ID: 2, TraceID: 2, Name: "foo", Duration: &d, Timestamp: &ts,
		BinaryAnnotations: []*zipkincore.BinaryAnnotation{
			{Key: zipkincore.LOCAL_COMPONENT, Host: &zipkincore.Endpoint{ServiceName: "bar", Ipv4: 170594602, Port: 8080},
				AnnotationType: zipkincore.AnnotationType_STRING},
		}}
	assert.Equal(t, tSpan, tSpans[0])
}

func loadProto(t *testing.T, fname string, spans *zmodel.ListOfSpans) {
	b, err := ioutil.ReadFile(fname)
	require.NoError(t, err)
	err = json.Unmarshal(b, spans)
	require.NoError(t, err)
}

func TestIdErrs(t *testing.T) {
	validID := randBytesOfLen(8)
	validTraceID := randBytesOfLen(16)
	invalidTraceID := randBytesOfLen(32)
	invalidParentID := randBytesOfLen(32)
	tests := []struct {
		span   zmodel.Span
		errMsg string
	}{
		{span: zmodel.Span{Id: randBytesOfLen(16)}, errMsg: "invalid Span ID"},
		{span: zmodel.Span{Id: validID, TraceId: invalidTraceID}, errMsg: "invalid traceId"},
		{span: zmodel.Span{Id: validID, TraceId: validTraceID, ParentId: invalidParentID}, errMsg: "invalid parentId"},
	}
	for _, test := range tests {
		_, err := protoSpanV2ToThrift(&test.span)
		require.Error(t, err)
		assert.Equal(t, err.Error(), test.errMsg)
	}
}

func TestEndpointValueErrs(t *testing.T) {
	validID := randBytesOfLen(8)
	validTraceID := randBytesOfLen(16)
	invalidLocalEp := zmodel.Endpoint{Ipv4: randBytesOfLen(8)}
	invalidRemoteEp := zmodel.Endpoint{Ipv6: randBytesOfLen(8)}
	tests := []struct {
		span   zmodel.Span
		errMsg string
	}{
		{span: zmodel.Span{Id: validID, TraceId: validTraceID, LocalEndpoint: &invalidLocalEp}, errMsg: "wrong Ipv4"},
		{span: zmodel.Span{Id: validID, TraceId: validTraceID, RemoteEndpoint: &invalidRemoteEp}, errMsg: "wrong Ipv6"},
	}
	for _, test := range tests {
		_, err := protoSpanV2ToThrift(&test.span)
		require.Error(t, err)
		assert.Equal(t, err.Error(), test.errMsg)
	}
}

func TestProtoKindToThrift(t *testing.T) {
	tests := []struct {
		ts       int64
		d        int64
		kind     zipkinProto.Span_Kind
		expected []*zipkincore.Annotation
	}{
		{kind: zipkinProto.Span_CLIENT, ts: 0, d: 1, expected: []*zipkincore.Annotation{{Value: zipkincore.CLIENT_SEND, Timestamp: 0}, {Value: zipkincore.CLIENT_RECV, Timestamp: 1}}},
		{kind: zipkinProto.Span_SERVER, ts: 0, d: 1, expected: []*zipkincore.Annotation{{Value: zipkincore.SERVER_RECV, Timestamp: 0}, {Value: zipkincore.SERVER_SEND, Timestamp: 1}}},
		{kind: zipkinProto.Span_PRODUCER, ts: 0, d: 1, expected: []*zipkincore.Annotation{{Value: zipkincore.MESSAGE_SEND, Timestamp: 0}}},
		{kind: zipkinProto.Span_CONSUMER, ts: 0, d: 1, expected: []*zipkincore.Annotation{{Value: zipkincore.MESSAGE_RECV, Timestamp: 0}}},
	}
	for _, test := range tests {
		banns := protoKindToThrift(test.ts, test.d, test.kind, nil)
		assert.Equal(t, banns, test.expected)
	}
}

func randBytesOfLen(n int) []byte {
	b := make([]byte, n)
	rand.Read(b)
	return b
}
