// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package zipkin

import (
	"encoding/json"
	"fmt"
	"math"
	"testing"

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
	spans, err := deserializeJSON([]byte(""))
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
	assert.Equal(t, "bar", binAnno.Value)
	assert.Equal(t, "foo", binAnno.Endpoint.ServiceName)
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
	spans, err := deserializeJSON([]byte(spanJSON))
	require.Error(t, err)
	assert.Equal(t, errIsNotUnsignedLog, err)
	assert.Nil(t, spans)
	// id longer than 32
	spanJSON = createSpan("bar", "123456789123456712345678912345678", "1", "2",
		156, 15145, false, "", "")
	spans, err = deserializeJSON([]byte(spanJSON))
	require.Error(t, err)
	assert.Equal(t, errIsNotUnsignedLog, err)
	assert.Nil(t, spans)
	// traceId missing
	spanJSON = createSpan("bar", "2", "1", "", 156, 15145, false,
		"", "")
	spans, err = deserializeJSON([]byte(spanJSON))
	require.Error(t, err)
	assert.Equal(t, errIsNotUnsignedLog, err)
	assert.Nil(t, spans)
	// 128 bit traceId
	spanJSON = createSpan("bar", "2", "1", "12345678912345671234567891234567", 156, 15145, false,
		"", "")
	spans, err = deserializeJSON([]byte(spanJSON))
	require.NoError(t, err)
	assert.NotNil(t, spans)
	// wrong 128 bit traceId
	spanJSON = createSpan("bar", "22", "12", "#2345678912345671234567891234562", 156, 15145, false,
		"", "")
	spans, err = deserializeJSON([]byte(spanJSON))
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
		ParentID:          "1",
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
		err  error
	}{
		{
			span: zipkinSpan{ID: "zd7a977555f6b982", TraceID: "bd7a977555f6b982"},
			err:  errIsNotUnsignedLog,
		},
		{
			span: zipkinSpan{ID: "ad7a977555f6b982", TraceID: "zd7a977555f6b982"},
			err:  errIsNotUnsignedLog,
		},
		{
			span: zipkinSpan{ID: "ad7a977555f6b982", TraceID: "ad7a977555f6b982", ParentID: "zd7a977555f6b982"},
			err:  errIsNotUnsignedLog,
		},
		{
			span: zipkinSpan{ID: "1", TraceID: "1", Annotations: []annotation{{Endpoint: endpoint{IPv4: "127.0.0.A"}}}},
			err:  errWrongIpv4,
		},
		{
			span: zipkinSpan{ID: "1", TraceID: "1", BinaryAnnotations: []binaryAnnotation{{Endpoint: endpoint{IPv4: "127.0.0.A"}}}},
			err:  errWrongIpv4,
		},
	}

	for _, test := range tests {
		tSpan, err = spanToThrift(test.span)
		require.Error(t, err)
		assert.Equal(t, test.err, err)
		assert.Nil(t, tSpan)
	}
}

func TestHexToUnsignedLong(t *testing.T) {
	okTests := []struct {
		hex      string
		expected uint64
	}{
		{hex: "0", expected: 0},
		{hex: "ffffffffffffffff", expected: math.MaxUint64},
		{hex: "00000000000000001", expected: 1},
	}
	for _, test := range okTests {
		num, err := hexToUnsignedLong(test.hex)
		require.NoError(t, err)
		assert.Equal(t, test.expected, num)
	}

	// drop higher bits
	num, err := hexToUnsignedLong("463ac35c9f6413ad48485a3953bb6124")
	num2, err2 := hexToUnsignedLong("48485a3953bb6124")
	require.NoError(t, err)
	require.NoError(t, err2)
	assert.Equal(t, num, num2)

	errTests := []struct {
		hex string
	}{
		{hex: "fffffffffffffffffffffffffffffffff"},
		{hex: ""},
		{hex: "po"},
	}
	for _, test := range errTests {
		num, err = hexToUnsignedLong(test.hex)
		require.Error(t, err)
		assert.Equal(t, errIsNotUnsignedLog, err)
		assert.Equal(t, uint64(0), num)
	}
}
