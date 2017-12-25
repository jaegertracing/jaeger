// Copyright (c) 2017 Uber Technologies, Inc.
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
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var endpointFmt = `{"serviceName": "%s", "ipv4": "%s", "ipv6": "%s", "port": %d}`
var annoFmt = `{"value": "%s", "timestamp": %d, "endpoint": %s}`
var binaAnnoFmt = `{"key": "%s", "value": "%s", "endpoint": %s}`
var spanFmt = `[{"name": "%s", "id": "%s", "parentId": "%s", "traceId": "%s", "timestamp": %d, "duration": %d, "debug": %t, "annotations": [%s], "binaryAnnotations": [%s]}]`

func createEndpoint(serviveName string, ipv4 string, ipv6 string, port int) string {
	return fmt.Sprintf(endpointFmt, serviveName, ipv4, ipv6, port)
}

func createAnno(val string, ts int, endpoint string) string {
	return fmt.Sprintf(annoFmt, val, ts, endpoint)
}

func createBinAnno(key string, val string, endpoint string) string {
	return fmt.Sprintf(binaAnnoFmt, key, val, endpoint)
}

func createSpan(name string, id string, parentID string, traceID string, ts int64, duration int64, debug bool,
	anno string, binAnno string) string {
	return fmt.Sprintf(spanFmt, name, id, parentID, traceID, ts, duration, debug, anno, binAnno)
}

func TestDecodeWrongJson(t *testing.T) {
	spans, err := DeserializeJSON([]byte(""))
	require.Error(t, err)
	assert.Nil(t, spans)
}

func TestUnmarshalEndpoint(t *testing.T) {
	endp := &endpoint{}
	err := json.Unmarshal([]byte(createEndpoint("foo", "127.0.0.1", "2001:db8::c001", 66)), endp)
	require.NoError(t, err)
	assert.Equal(t, "foo", endp.ServiceName)
	assert.Equal(t, "127.0.0.1", endp.IPv4)
	assert.Equal(t, "2001:db8::c001", endp.IPv6)
}

func TestUnmarshalAnnotation(t *testing.T) {
	anno := &annotation{}
	endpointJSON := createEndpoint("foo", "127.0.0.1", "2001:db8::c001", 66)
	err := json.Unmarshal([]byte(createAnno("bar", 154, endpointJSON)), anno)
	require.NoError(t, err)
	assert.Equal(t, "bar", anno.Value)
	assert.Equal(t, int64(154), anno.Timestamp)
	assert.Equal(t, "foo", anno.Endpoint.ServiceName)
}

func TestUnmarshalBinAnnotation(t *testing.T) {
	binAnno := &binaryAnnotation{}
	endpointJSON := createEndpoint("foo", "127.0.0.1", "2001:db8::c001", 66)
	err := json.Unmarshal([]byte(createBinAnno("foo", "bar", endpointJSON)), binAnno)
	require.NoError(t, err)
	assert.Equal(t, "foo", binAnno.Key)
	assert.Equal(t, "bar", binAnno.Value.(string))
	assert.Equal(t, "foo", binAnno.Endpoint.ServiceName)
}

func TestUnmarshalBinAnnotationNumberValue(t *testing.T) {
	tests := []struct {
		json     string
		expected zipkincore.BinaryAnnotation
		err      error
	}{
		{
			json:     `{"key":"foo", "value": 32768, "type": "I16"}`,
			expected: zipkincore.BinaryAnnotation{Key: "foo", Value: []byte{0x0, 0x80}, AnnotationType: zipkincore.AnnotationType_I16},
		},
		{
			json:     `{"key":"foo", "value": 32768, "type": "I32"}`,
			expected: zipkincore.BinaryAnnotation{Key: "foo", Value: []byte{0x00, 0x80, 0x00, 0x00}, AnnotationType: zipkincore.AnnotationType_I32},
		},
		{
			json:     `{"key":"foo", "value": 32768, "type": "I64"}`,
			expected: zipkincore.BinaryAnnotation{Key: "foo", Value: []byte{0x00, 0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, AnnotationType: zipkincore.AnnotationType_I64},
		},
		{
			json:     `{"key":"foo", "value": -12.666512, "type": "DOUBLE"}`,
			expected: zipkincore.BinaryAnnotation{Key: "foo", Value: []byte{122, 200, 148, 15, 65, 85, 41, 192}, AnnotationType: zipkincore.AnnotationType_DOUBLE},
		},
		{
			json:     `{"key":"foo", "value": true, "type": "BOOL"}`,
			expected: zipkincore.BinaryAnnotation{Key: "foo", Value: []byte{1}, AnnotationType: zipkincore.AnnotationType_BOOL},
		},
		{
			json:     `{"key":"foo", "value": false, "type": "BOOL"}`,
			expected: zipkincore.BinaryAnnotation{Key: "foo", Value: []byte{0}, AnnotationType: zipkincore.AnnotationType_BOOL},
		},
		{
			json:     `{"key":"foo", "value": "str", "type": "STRING"}`,
			expected: zipkincore.BinaryAnnotation{Key: "foo", Value: []byte("str"), AnnotationType: zipkincore.AnnotationType_STRING},
		},
		{
			json:     `{"key":"foo", "value": "c3Ry", "type": "BYTES"}`,
			expected: zipkincore.BinaryAnnotation{Key: "foo", Value: []byte("str"), AnnotationType: zipkincore.AnnotationType_BYTES},
		},
		{
			json: `{"key":"foo", "value": "^^^", "type": "BYTES"}`,
			err:  errors.New("illegal base64 data at input byte 0"),
		},
		{
			json:     `{"key":"foo", "value": "733c374d736e41cc"}`,
			expected: zipkincore.BinaryAnnotation{Key: "foo", Value: []byte("733c374d736e41cc"), AnnotationType: zipkincore.AnnotationType_STRING},
		},
	}

	for _, test := range tests {
		binAnno := &binaryAnnotation{}
		err := json.Unmarshal([]byte(test.json), binAnno)
		require.NoError(t, err)
		tBinAnno, err := binAnnoToThrift(*binAnno)
		if test.err != nil {
			require.Error(t, err, test.json)
			require.Nil(t, tBinAnno)
			assert.Equal(t, test.err.Error(), err.Error())
		} else {
			require.NoError(t, err)
			assert.Equal(t, test.expected.Key, tBinAnno.Key)
			assert.Equal(t, test.expected.Value, tBinAnno.Value)
		}
	}
}

func TestUnmarshalSpan(t *testing.T) {
	endpJSON := createEndpoint("foo", "127.0.0.1", "2001:db8::c001", 66)
	annoJSON := createAnno("cs", 1515, endpJSON)
	binAnnoJSON := createBinAnno("http.status_code", "200", endpJSON)
	spanJSON := createSpan("bar", "1234567891234567", "1234567891234567", "1234567891234567", 156, 15145, false,
		annoJSON, binAnnoJSON)

	spans, err := decode([]byte(spanJSON))
	require.NoError(t, err)
	assert.NotNil(t, spans)
	assert.Equal(t, 1, len(spans))
	assert.Equal(t, "bar", spans[0].Name)
	assert.Equal(t, false, spans[0].Debug)
	assert.Equal(t, "1234567891234567", spans[0].ParentID)
	assert.Equal(t, "1234567891234567", spans[0].TraceID)
	assert.Equal(t, "1234567891234567", spans[0].ID)
	assert.Equal(t, int64(15145), *spans[0].Duration)
	assert.Equal(t, int64(156), *spans[0].Timestamp)
	assert.Equal(t, 1, len(spans[0].Annotations))
	assert.Equal(t, 1, len(spans[0].BinaryAnnotations))

	spans, err = decode([]byte(createSpan("bar", "1234567891234567", "1234567891234567", "1234567891234567",
		156, 15145, false, "", "")))
	require.NoError(t, err)
	assert.Equal(t, 1, len(spans))
	assert.Equal(t, 0, len(spans[0].Annotations))
	assert.Equal(t, 0, len(spans[0].BinaryAnnotations))
}

func TestIncorrectSpanIds(t *testing.T) {
	// id missing
	spanJSON := createSpan("bar", "", "1", "2", 156, 15145, false, "", "")
	spans, err := DeserializeJSON([]byte(spanJSON))
	require.Error(t, err)
	assert.Equal(t, "strconv.ParseUint: parsing \"\": invalid syntax", err.Error())
	assert.Nil(t, spans)
	// id longer than 32
	spanJSON = createSpan("bar", "123456789123456712345678912345678", "1", "2",
		156, 15145, false, "", "")
	spans, err = DeserializeJSON([]byte(spanJSON))
	require.Error(t, err)
	assert.Equal(t, "SpanID cannot be longer than 16 hex characters: 123456789123456712345678912345678", err.Error())
	assert.Nil(t, spans)
	// traceId missing
	spanJSON = createSpan("bar", "2", "1", "", 156, 15145, false,
		"", "")
	spans, err = DeserializeJSON([]byte(spanJSON))
	require.Error(t, err)
	assert.Equal(t, "strconv.ParseUint: parsing \"\": invalid syntax", err.Error())
	assert.Nil(t, spans)
	// 128 bit traceId
	spanJSON = createSpan("bar", "2", "1", "12345678912345671234567891234567", 156, 15145, false,
		"", "")
	spans, err = DeserializeJSON([]byte(spanJSON))
	require.NoError(t, err)
	assert.NotNil(t, spans)
	// wrong 128 bit traceId
	spanJSON = createSpan("bar", "22", "12", "#2345678912345671234567891234562", 156, 15145, false,
		"", "")
	spans, err = DeserializeJSON([]byte(spanJSON))
	require.Error(t, err)
	assert.Nil(t, spans)
}

func TestEndpointToThrift(t *testing.T) {
	endp := endpoint{
		ServiceName: "foo",
		Port:        80,
		IPv4:        "127.0.0.1",
	}
	tEndpoint, err := endpointToThrift(endp)
	require.NoError(t, err)
	assert.Equal(t, "foo", tEndpoint.ServiceName)
	assert.Equal(t, int16(80), tEndpoint.Port)
	assert.Equal(t, int32(2130706433), tEndpoint.Ipv4)

	endp = endpoint{
		ServiceName: "foo",
		Port:        80,
		IPv4:        "",
	}
	tEndpoint, err = endpointToThrift(endp)
	require.NoError(t, err)
	assert.Equal(t, "foo", tEndpoint.ServiceName)
	assert.Equal(t, int16(80), tEndpoint.Port)
	assert.Equal(t, int32(0), tEndpoint.Ipv4)

	endp = endpoint{
		ServiceName: "foo",
		Port:        80,
		IPv4:        "127.0.0.A",
	}
	tEndpoint, err = endpointToThrift(endp)
	require.Error(t, err)
	assert.Equal(t, errWrongIpv4, err)
	assert.Nil(t, tEndpoint)

	endp = endpoint{
		ServiceName: "foo",
		Port:        80,
		IPv6:        "::R",
	}
	tEndpoint, err = endpointToThrift(endp)
	require.Error(t, err)
	assert.Equal(t, errWrongIpv6, err)
	assert.Nil(t, tEndpoint)
}

func TestAnnotationToThrift(t *testing.T) {
	endp := endpoint{
		ServiceName: "foo",
		Port:        80,
		IPv4:        "127.0.0.1",
	}
	anno := annotation{
		Value:     "cs",
		Timestamp: 152,
		Endpoint:  endp,
	}
	tAnno, err := annoToThrift(anno)
	require.NoError(t, err)
	assert.Equal(t, anno.Value, tAnno.Value)
	assert.Equal(t, anno.Timestamp, tAnno.Timestamp)
	assert.Equal(t, anno.Endpoint.ServiceName, tAnno.Host.ServiceName)

	endp = endpoint{
		IPv4: "127.0.0.A",
	}
	anno = annotation{
		Endpoint: endp,
	}
	tAnno, err = annoToThrift(anno)
	require.Error(t, err)
	assert.Equal(t, errWrongIpv4, err)
	assert.Nil(t, tAnno)
}

func TestBinaryAnnotationToThrift(t *testing.T) {
	endp := endpoint{
		ServiceName: "foo",
		Port:        80,
		IPv4:        "127.0.0.1",
	}
	binAnno := binaryAnnotation{
		Endpoint: endp,
		Key:      "error",
		Value:    "str",
		Type:     "STRING",
	}
	tBinAnno, err := binAnnoToThrift(binAnno)
	require.NoError(t, err)
	assert.Equal(t, binAnno.Key, tBinAnno.Key)
	assert.Equal(t, binAnno.Endpoint.ServiceName, tBinAnno.Host.ServiceName)
	assert.Equal(t, binAnno.Value, string(tBinAnno.Value))

	endp = endpoint{
		IPv4: "127.0.0.A",
	}
	binAnno = binaryAnnotation{
		Endpoint: endp,
	}
	tBinAnno, err = binAnnoToThrift(binAnno)
	require.Error(t, err)
	assert.Nil(t, tBinAnno)
}

func TestSpanToThrift(t *testing.T) {
	endp := endpoint{
		ServiceName: "foo",
		Port:        80,
		IPv4:        "127.0.0.1",
	}
	anno := annotation{
		Value:     "cs",
		Timestamp: 152,
		Endpoint:  endp,
	}
	binAnno := binaryAnnotation{
		Endpoint: endp,
		Key:      "error",
		Value:    "str",
	}
	span := zipkinSpan{
		ID:                "bd7a977555f6b982",
		TraceID:           "bd7a974555f6b982bd71977555f6b981",
		ParentID:          "00000000000000001",
		Name:              "foo",
		Annotations:       []annotation{anno},
		BinaryAnnotations: []binaryAnnotation{binAnno},
	}
	tSpan, err := spanToThrift(span)
	require.NoError(t, err)
	assert.Equal(t, int64(-4795885597963667071), tSpan.TraceID)
	assert.Equal(t, int64(-4793352529331701374), *tSpan.TraceIDHigh)
	assert.Equal(t, int64(-4793352323173271166), tSpan.ID)
	assert.Equal(t, int64(1), *tSpan.ParentID)

	assert.Equal(t, span.Name, tSpan.Name)
	assert.Equal(t, anno.Value, tSpan.Annotations[0].Value)
	assert.Equal(t, anno.Endpoint.ServiceName, tSpan.Annotations[0].Host.ServiceName)
	assert.Equal(t, binAnno.Key, tSpan.BinaryAnnotations[0].Key)
	assert.Equal(t, binAnno.Endpoint.ServiceName, tSpan.BinaryAnnotations[0].Host.ServiceName)

	tests := []struct {
		span zipkinSpan
		err  string
	}{
		{
			span: zipkinSpan{ID: "zd7a977555f6b982", TraceID: "bd7a977555f6b982"},
			err:  "strconv.ParseUint: parsing \"zd7a977555f6b982\": invalid syntax",
		},
		{
			span: zipkinSpan{ID: "ad7a977555f6b982", TraceID: "zd7a977555f6b982"},
			err:  "strconv.ParseUint: parsing \"zd7a977555f6b982\": invalid syntax",
		},
		{
			span: zipkinSpan{ID: "ad7a977555f6b982", TraceID: "ad7a977555f6b982", ParentID: "zd7a977555f6b982"},
			err:  "strconv.ParseUint: parsing \"zd7a977555f6b982\": invalid syntax",
		},
		{
			span: zipkinSpan{ID: "1", TraceID: "1", Annotations: []annotation{{Endpoint: endpoint{IPv4: "127.0.0.A"}}}},
			err:  errWrongIpv4.Error(),
		},
		{
			span: zipkinSpan{ID: "1", TraceID: "1", BinaryAnnotations: []binaryAnnotation{{Endpoint: endpoint{IPv4: "127.0.0.A"}}}},
			err:  errWrongIpv4.Error(),
		},
	}

	for _, test := range tests {
		tSpan, err = spanToThrift(test.span)
		require.Error(t, err)
		assert.Equal(t, test.err, err.Error())
		assert.Nil(t, tSpan)
	}
}
