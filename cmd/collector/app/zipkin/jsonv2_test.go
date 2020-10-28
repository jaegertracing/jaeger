// Copyright (c) 2017 The Jaeger Authors.
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
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/go-openapi/swag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/swagger-gen/models"
	"github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
)

func TestFixtures(t *testing.T) {
	var spans models.ListOfSpans
	loadJSON(t, "fixtures/zipkin_01.json", &spans)
	tSpans, err := spansV2ToThrift(spans)
	require.NoError(t, err)
	assert.Equal(t, len(tSpans), 1)
	var pid int64 = 1
	var ts int64 = 1
	var d int64 = 10
	localE := &zipkincore.Endpoint{ServiceName: "foo", Ipv4: 170594602}
	remoteE := &zipkincore.Endpoint{ServiceName: "bar", Ipv4: 170594603}
	var highID = int64(-4793352529331701374)
	tSpan := &zipkincore.Span{ID: 2, TraceID: int64(-4795885597963667071), TraceIDHigh: &highID, ParentID: &pid, Name: "foo", Debug: true, Duration: &d, Timestamp: &ts,
		Annotations: []*zipkincore.Annotation{
			{Value: "foo", Timestamp: 1, Host: localE},
			{Value: zipkincore.CLIENT_SEND, Timestamp: ts, Host: localE},
			{Value: zipkincore.CLIENT_RECV, Timestamp: ts + d, Host: localE}},
		BinaryAnnotations: []*zipkincore.BinaryAnnotation{
			{Key: "foo", Value: []byte("bar"), Host: localE, AnnotationType: zipkincore.AnnotationType_STRING},
			{Key: zipkincore.SERVER_ADDR, Host: remoteE, AnnotationType: zipkincore.AnnotationType_BOOL}}}
	assert.Equal(t, tSpan, tSpans[0])
}

func TestLCFromLocalEndpoint(t *testing.T) {
	var spans models.ListOfSpans
	loadJSON(t, "fixtures/zipkin_02.json", &spans)
	tSpans, err := spansV2ToThrift(spans)
	fmt.Println(tSpans[0])
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

func TestKindToThrift(t *testing.T) {
	tests := []struct {
		ts       int64
		d        int64
		kind     string
		expected []*zipkincore.Annotation
	}{
		{kind: models.SpanKindCLIENT, ts: 0, d: 1, expected: []*zipkincore.Annotation{{Value: zipkincore.CLIENT_SEND, Timestamp: 0}, {Value: zipkincore.CLIENT_RECV, Timestamp: 1}}},
		{kind: models.SpanKindSERVER, ts: 0, d: 1, expected: []*zipkincore.Annotation{{Value: zipkincore.SERVER_RECV, Timestamp: 0}, {Value: zipkincore.SERVER_SEND, Timestamp: 1}}},
		{kind: models.SpanKindPRODUCER, ts: 0, d: 1, expected: []*zipkincore.Annotation{{Value: zipkincore.MESSAGE_SEND, Timestamp: 0}}},
		{kind: models.SpanKindCONSUMER, ts: 0, d: 1, expected: []*zipkincore.Annotation{{Value: zipkincore.MESSAGE_RECV, Timestamp: 0}}},
	}
	for _, test := range tests {
		banns := kindToThrift(test.ts, test.d, test.kind, nil)
		assert.Equal(t, banns, test.expected)
	}
}

func TestRemoteEndpToThrift(t *testing.T) {
	tests := []struct {
		kind     string
		expected *zipkincore.BinaryAnnotation
	}{
		{kind: models.SpanKindCLIENT, expected: &zipkincore.BinaryAnnotation{Key: zipkincore.SERVER_ADDR, AnnotationType: zipkincore.AnnotationType_BOOL}},
		{kind: models.SpanKindSERVER, expected: &zipkincore.BinaryAnnotation{Key: zipkincore.CLIENT_ADDR, AnnotationType: zipkincore.AnnotationType_BOOL}},
		{kind: models.SpanKindPRODUCER, expected: &zipkincore.BinaryAnnotation{Key: zipkincore.MESSAGE_ADDR, AnnotationType: zipkincore.AnnotationType_BOOL}},
		{kind: models.SpanKindCONSUMER, expected: &zipkincore.BinaryAnnotation{Key: zipkincore.MESSAGE_ADDR, AnnotationType: zipkincore.AnnotationType_BOOL}},
		{kind: "", expected: nil},
	}
	for _, test := range tests {
		banns, err := remoteEndpToThrift(nil, test.kind)
		require.NoError(t, err)
		assert.Equal(t, banns, test.expected)
	}
}

func TestErrIds(t *testing.T) {
	idOk := "a"
	idWrong := "z"
	tests := []struct {
		span models.Span
	}{
		{span: models.Span{ID: &idWrong}},
		{span: models.Span{ID: &idOk, TraceID: &idWrong}},
		{span: models.Span{ID: &idOk, TraceID: &idOk, ParentID: idWrong}},
	}
	for _, test := range tests {
		tSpan, err := spanV2ToThrift(&test.span)
		require.Error(t, err)
		require.Nil(t, tSpan)
		assert.Equal(t, err.Error(), "strconv.ParseUint: parsing \"z\": invalid syntax")
	}
}

func TestErrEndpoints(t *testing.T) {
	id := "A"
	endp := models.Endpoint{IPV4: "192.168.0.0.1"}
	tests := []struct {
		span models.Span
	}{
		{span: models.Span{ID: &id, TraceID: &id, LocalEndpoint: &endp}},
		{span: models.Span{ID: &id, TraceID: &id, RemoteEndpoint: &endp}},
	}
	for _, test := range tests {
		tSpan, err := spanV2ToThrift(&test.span)
		require.Error(t, err)
		require.Nil(t, tSpan)
		assert.Equal(t, err.Error(), "wrong ipv4")
	}
}

func TestErrSpans(t *testing.T) {
	id := "z"
	tSpans, err := spansV2ToThrift(models.ListOfSpans{&models.Span{ID: &id}})
	require.Error(t, err)
	require.Nil(t, tSpans)
	assert.Equal(t, err.Error(), "strconv.ParseUint: parsing \"z\": invalid syntax")
}

func loadJSON(t *testing.T, fileName string, i interface{}) {
	b, err := ioutil.ReadFile(fileName)
	require.NoError(t, err)
	err = swag.ReadJSON(b, i)
	require.NoError(t, err, "Failed to parse json fixture file %s", fileName)
}
