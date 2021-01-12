// Copyright (c) 2019 The Jaeger Authors.
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
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	jaegerClient "github.com/uber/jaeger-client-go"
	zipkinTransport "github.com/uber/jaeger-client-go/transport/zipkin"

	"github.com/jaegertracing/jaeger/cmd/collector/app/handler"
	zipkinTrift "github.com/jaegertracing/jaeger/model/converter/thrift/zipkin"
	zipkinProto "github.com/jaegertracing/jaeger/proto-gen/zipkin"
	"github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
)

var httpClient = &http.Client{Timeout: 2 * time.Second}

type mockZipkinHandler struct {
	err   error
	mux   sync.Mutex
	spans []*zipkincore.Span
}

func (p *mockZipkinHandler) SubmitZipkinBatch(spans []*zipkincore.Span, opts handler.SubmitBatchOptions) ([]*zipkincore.Response, error) {
	p.mux.Lock()
	defer p.mux.Unlock()
	p.spans = append(p.spans, spans...)
	return nil, p.err
}

func (p *mockZipkinHandler) getSpans() []*zipkincore.Span {
	p.mux.Lock()
	defer p.mux.Unlock()
	return p.spans
}

func initializeTestServer(err error) (*httptest.Server, *APIHandler) {
	r := mux.NewRouter()
	handler := NewAPIHandler(&mockZipkinHandler{err: err})
	handler.RegisterRoutes(r)
	return httptest.NewServer(r), handler
}

func TestViaClient(t *testing.T) {
	server, handler := initializeTestServer(nil)
	defer server.Close()

	zipkinSender, err := zipkinTransport.NewHTTPTransport(
		server.URL+`/api/v1/spans`,
		zipkinTransport.HTTPBatchSize(1),
	)
	require.NoError(t, err)

	tracer, closer := jaegerClient.NewTracer(
		"test",
		jaegerClient.NewConstSampler(true),
		jaegerClient.NewRemoteReporter(zipkinSender),
	)
	defer closer.Close()

	tracer.StartSpan("root").Finish()

	waitForSpans(t, handler.zipkinSpansHandler.(*mockZipkinHandler), 1)
}

func waitForSpans(t *testing.T, handler *mockZipkinHandler, expecting int) {
	deadline := time.Now().Add(2 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Error("never received a span")
			return
		}
		if have := len(handler.getSpans()); expecting != have {
			time.Sleep(time.Millisecond)
			continue
		}
		break
	}
	assert.Len(t, handler.getSpans(), expecting)
}

func TestThriftFormat(t *testing.T) {
	server, _ := initializeTestServer(nil)
	defer server.Close()
	bodyBytes := zipkinTrift.SerializeThrift([]*zipkincore.Span{{}})
	statusCode, resBodyStr, err := postBytes(server.URL+`/api/v1/spans`, bodyBytes, createHeader("application/x-thrift"))
	assert.NoError(t, err)
	assert.EqualValues(t, http.StatusAccepted, statusCode)
	assert.EqualValues(t, "", resBodyStr)
}

func TestJsonFormat(t *testing.T) {
	server, handler := initializeTestServer(nil)
	defer server.Close()

	endpJSON := createEndpoint("foo", "127.0.0.1", "2001:db8::c001", 65535)
	annoJSON := createAnno("cs", 1515, endpJSON)
	binAnnoJSON := createBinAnno("http.status_code", "200", endpJSON)
	spanJSON := createSpan("bar", "1234567891234565", "1234567891234567", "1234567891234568", 156, 15145, false,
		annoJSON, binAnnoJSON)
	statusCode, resBodyStr, err := postBytes(server.URL+`/api/v1/spans`, []byte(spanJSON), createHeader("application/json"))
	assert.NoError(t, err)
	assert.EqualValues(t, http.StatusAccepted, statusCode)
	assert.EqualValues(t, "", resBodyStr)
	waitForSpans(t, handler.zipkinSpansHandler.(*mockZipkinHandler), 1)
	recdSpan := handler.zipkinSpansHandler.(*mockZipkinHandler).getSpans()[0]
	require.Len(t, recdSpan.Annotations, 1)
	require.NotNil(t, recdSpan.Annotations[0].Host)
	assert.EqualValues(t, -1, recdSpan.Annotations[0].Host.Port, "Port 65535 must be represented as -1 in zipkin.thrift")

	statusCode, resBodyStr, err = postBytes(server.URL+`/api/v1/spans`, []byte(spanJSON), createHeader("application/json; charset=utf-8"))
	assert.NoError(t, err)
	assert.EqualValues(t, http.StatusAccepted, statusCode)
	assert.EqualValues(t, "", resBodyStr)

	endpErrJSON := createEndpoint("", "127.0.0.A", "", 80)

	// error zipkinSpanHandler
	handler.zipkinSpansHandler.(*mockZipkinHandler).err = fmt.Errorf("Bad times ahead")
	tests := []struct {
		payload    string
		expected   string
		statusCode int
	}{
		{
			payload:    spanJSON,
			expected:   "Cannot submit Zipkin batch: Bad times ahead\n",
			statusCode: http.StatusInternalServerError,
		},
		{
			payload:    createSpan("bar", "", "1", "1", 156, 15145, false, annoJSON, binAnnoJSON),
			expected:   "Unable to process request body: strconv.ParseUint: parsing &#34;&#34;: invalid syntax\n",
			statusCode: http.StatusBadRequest,
		},
		{
			payload:    createSpan("bar", "ZTA", "1", "1", 156, 15145, false, "", ""),
			expected:   "Unable to process request body: strconv.ParseUint: parsing &#34;ZTA&#34;: invalid syntax\n",
			statusCode: http.StatusBadRequest,
		},
		{
			payload:    createSpan("bar", "1", "", "1", 156, 15145, false, "", createAnno("cs", 1, endpErrJSON)),
			expected:   "Unable to process request body: wrong ipv4\n",
			statusCode: http.StatusBadRequest,
		},
	}

	for _, test := range tests {
		statusCode, resBodyStr, err = postBytes(server.URL+`/api/v1/spans`, []byte(test.payload), createHeader("application/json"))
		require.NoError(t, err)
		assert.EqualValues(t, test.statusCode, statusCode)
		assert.EqualValues(t, test.expected, resBodyStr)
	}
}

func TestGzipEncoding(t *testing.T) {
	server, _ := initializeTestServer(nil)
	defer server.Close()
	bodyBytes := zipkinTrift.SerializeThrift([]*zipkincore.Span{{}})
	header := createHeader("application/x-thrift")
	header.Add("Content-Encoding", "gzip")
	statusCode, resBodyStr, err := postBytes(server.URL+`/api/v1/spans`, gzipEncode(bodyBytes), header)
	assert.NoError(t, err)
	assert.EqualValues(t, http.StatusAccepted, statusCode)
	assert.EqualValues(t, "", resBodyStr)
}

func TestGzipBadBody(t *testing.T) {
	server, _ := initializeTestServer(nil)
	defer server.Close()
	header := createHeader("")
	header.Add("Content-Encoding", "gzip")
	statusCode, resBodyStr, err := postBytes(server.URL+`/api/v1/spans`, []byte("not good"), header)
	assert.NoError(t, err)
	assert.EqualValues(t, http.StatusBadRequest, statusCode)
	assert.EqualValues(t, "Unable to process request body: unexpected EOF\n", resBodyStr)
}

func TestMalformedContentType(t *testing.T) {
	server, _ := initializeTestServer(nil)
	defer server.Close()
	statusCode, resBodyStr, err := postBytes(server.URL+`/api/v1/spans`, []byte{}, createHeader("application/json; =iammalformed;"))
	assert.NoError(t, err)
	assert.EqualValues(t, http.StatusBadRequest, statusCode)
	assert.EqualValues(t, "Cannot parse Content-Type: mime: invalid media parameter\n", resBodyStr)
}

func TestUnsupportedContentType(t *testing.T) {
	server, _ := initializeTestServer(nil)
	defer server.Close()
	statusCode, _, err := postBytes(server.URL+`/api/v1/spans`, []byte{}, createHeader("text/html"))
	assert.NoError(t, err)
	assert.EqualValues(t, http.StatusBadRequest, statusCode)
}

func TestFormatBadBody(t *testing.T) {
	server, _ := initializeTestServer(nil)
	defer server.Close()
	statusCode, resBodyStr, err := postBytes(server.URL+`/api/v1/spans`, []byte("not good"), createHeader("application/x-thrift"))
	assert.NoError(t, err)
	assert.EqualValues(t, http.StatusBadRequest, statusCode)
	assert.EqualValues(t, "Unable to process request body: Unknown data type 111\n", resBodyStr)
}

func TestCannotReadBodyFromRequest(t *testing.T) {
	handler := NewAPIHandler(&mockZipkinHandler{})
	req, err := http.NewRequest(http.MethodPost, "whatever", &errReader{})
	assert.NoError(t, err)
	rw := dummyResponseWriter{}

	tests := []struct {
		handler func(w http.ResponseWriter, r *http.Request)
	}{
		{handler: handler.saveSpans},
		{handler: handler.saveSpansV2},
	}
	for _, test := range tests {
		test.handler(&rw, req)
		assert.EqualValues(t, http.StatusInternalServerError, rw.myStatusCode)
		assert.EqualValues(t, "Unable to process request body: Simulated error reading body\n", rw.myBody)
	}
}

func TestSaveSpansV2(t *testing.T) {
	server, handler := initializeTestServer(nil)
	defer server.Close()
	tests := []struct {
		body    []byte
		resBody string
		code    int
		headers map[string]string
	}{
		{body: []byte("[]"), code: http.StatusAccepted},
		{body: []byte("[]"), code: http.StatusAccepted, headers: map[string]string{"Content-Type": "application/json; charset=utf-8"}},
		{body: gzipEncode([]byte("[]")), code: http.StatusAccepted, headers: map[string]string{"Content-Encoding": "gzip"}},
		{body: []byte("[]"), code: http.StatusBadRequest, headers: map[string]string{"Content-Type": "text/html"}, resBody: "Unsupported Content-Type\n"},
		{body: []byte("[]"), code: http.StatusBadRequest, headers: map[string]string{"Content-Type": "application/json; =iammalformed;"}, resBody: "Cannot parse Content-Type: mime: invalid media parameter\n"},
		{body: []byte("[]"), code: http.StatusBadRequest, headers: map[string]string{"Content-Encoding": "gzip"}, resBody: "Unable to process request body: unexpected EOF\n"},
		{body: []byte("not good"), code: http.StatusBadRequest, resBody: "Unable to process request body: invalid character 'o' in literal null (expecting 'u')\n"},
		{body: []byte("[{}]"), code: http.StatusBadRequest, resBody: "Unable to process request body: validation failure list:\nid in body is required\ntraceId in body is required\n"},
		{body: []byte(`[{"id":"1111111111111111", "traceId":"1111111111111111", "localEndpoint": {"ipv4": "A"}}]`), code: http.StatusBadRequest, resBody: "Unable to process request body: wrong ipv4\n"},
	}
	for _, test := range tests {
		h := createHeader("application/json")
		for k, v := range test.headers {
			h.Set(k, v)
		}
		statusCode, resBody, err := postBytes(server.URL+`/api/v2/spans`, test.body, h)
		require.NoError(t, err)
		assert.EqualValues(t, test.code, statusCode)
		assert.EqualValues(t, test.resBody, resBody)
	}
	handler.zipkinSpansHandler.(*mockZipkinHandler).err = fmt.Errorf("Bad times ahead")
	statusCode, resBody, err := postBytes(server.URL+`/api/v2/spans`, []byte(`[{"id":"1111111111111111", "traceId":"1111111111111111"}]`), createHeader("application/json"))
	require.NoError(t, err)
	assert.EqualValues(t, http.StatusInternalServerError, statusCode)
	assert.EqualValues(t, "Cannot submit Zipkin batch: Bad times ahead\n", resBody)
}

func TestSaveProtoSpansV2(t *testing.T) {
	server, handler := initializeTestServer(nil)
	defer server.Close()

	validID := randBytesOfLen(8)
	validTraceID := randBytesOfLen(16)
	tests := []struct {
		Span       zipkinProto.Span
		StatusCode int
		resBody    string
	}{
		{Span: zipkinProto.Span{Id: validID, TraceId: validTraceID, LocalEndpoint: &zipkinProto.Endpoint{Ipv4: randBytesOfLen(4)}, Kind: zipkinProto.Span_CLIENT}, StatusCode: http.StatusAccepted},
		{Span: zipkinProto.Span{Id: randBytesOfLen(4)}, StatusCode: http.StatusBadRequest, resBody: "Unable to process request body: invalid length for SpanID\n"},
		{Span: zipkinProto.Span{Id: validID, TraceId: randBytesOfLen(32)}, StatusCode: http.StatusBadRequest, resBody: "Unable to process request body: invalid length for TraceID\n"},
		{Span: zipkinProto.Span{Id: validID, TraceId: validTraceID, ParentId: randBytesOfLen(16)}, StatusCode: http.StatusBadRequest, resBody: "Unable to process request body: invalid length for SpanID\n"},
		{Span: zipkinProto.Span{Id: validID, TraceId: validTraceID, LocalEndpoint: &zipkinProto.Endpoint{Ipv4: randBytesOfLen(2)}}, StatusCode: http.StatusBadRequest, resBody: "Unable to process request body: wrong Ipv4\n"},
	}
	for _, test := range tests {
		l := zipkinProto.ListOfSpans{
			Spans: []*zipkinProto.Span{&test.Span},
		}
		reqBytes, _ := proto.Marshal(&l)
		statusCode, resBody, err := postBytes(server.URL+`/api/v2/spans`, reqBytes, createHeader("application/x-protobuf"))
		require.NoError(t, err)
		assert.EqualValues(t, test.StatusCode, statusCode)
		assert.EqualValues(t, test.resBody, resBody)
	}

	l := zipkinProto.ListOfSpans{}
	reqBytes, _ := proto.Marshal(&l)
	statusCode, _, err := postBytes(server.URL+`/api/v2/spans`, reqBytes, createHeader("application/x-protobuf"))
	require.NoError(t, err)
	assert.EqualValues(t, http.StatusAccepted, statusCode)

	invalidSpans := struct{ Key string }{Key: "foo"}
	reqBytes, _ = json.Marshal(&invalidSpans)
	statusCode, resBody, err := postBytes(server.URL+`/api/v2/spans`, reqBytes, createHeader("application/x-protobuf"))
	require.NoError(t, err)
	assert.EqualValues(t, http.StatusBadRequest, statusCode)
	assert.EqualValues(t, "Unable to process request body: unexpected EOF\n", resBody)

	reqBytes, _ = proto.Marshal(&zipkinProto.ListOfSpans{Spans: []*zipkinProto.Span{{Id: validID, TraceId: validTraceID}}})
	handler.zipkinSpansHandler.(*mockZipkinHandler).err = fmt.Errorf("Bad times ahead")
	statusCode, resBody, err = postBytes(server.URL+`/api/v2/spans`, reqBytes, createHeader("application/x-protobuf"))
	require.NoError(t, err)
	assert.EqualValues(t, http.StatusInternalServerError, statusCode)
	assert.EqualValues(t, "Cannot submit Zipkin batch: Bad times ahead\n", resBody)
}

type errReader struct{}

func (e *errReader) Read(p []byte) (int, error) {
	return 0, fmt.Errorf("Simulated error reading body")
}

type dummyResponseWriter struct {
	myBody       string
	myStatusCode int
}

func (d *dummyResponseWriter) Header() http.Header {
	return http.Header{}
}

func (d *dummyResponseWriter) Write(bodyBytes []byte) (int, error) {
	d.myBody = string(bodyBytes)
	return 0, nil
}

func (d *dummyResponseWriter) WriteHeader(statusCode int) {
	d.myStatusCode = statusCode
}

func createHeader(contentType string) *http.Header {
	header := &http.Header{}
	if len(contentType) > 0 {
		header.Add("Content-Type", contentType)
	}
	return header
}

func gzipEncode(b []byte) []byte {
	buffer := &bytes.Buffer{}
	z := gzip.NewWriter(buffer)
	z.Write(b)
	z.Close()
	return buffer.Bytes()
}

func postBytes(urlStr string, bytesBody []byte, header *http.Header) (int, string, error) {
	req, err := http.NewRequest(http.MethodPost, urlStr, bytes.NewBuffer(bytesBody))
	if err != nil {
		return 0, "", err
	}

	if header != nil {
		for name, values := range *header {
			for _, value := range values {
				req.Header.Add(name, value)
			}
		}
	}

	res, err := httpClient.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return 0, "", err
	}
	return res.StatusCode, string(body), nil
}
