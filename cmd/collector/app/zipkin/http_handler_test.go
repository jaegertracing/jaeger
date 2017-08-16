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
	"bytes"
	"compress/gzip"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/apache/thrift/lib/go/thrift"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	jaegerClient "github.com/uber/jaeger-client-go"
	zipkinTransport "github.com/uber/jaeger-client-go/transport/zipkin"
	tchanThrift "github.com/uber/tchannel-go/thrift"

	"github.com/uber/jaeger/thrift-gen/zipkincore"
)

var httpClient = &http.Client{Timeout: 2 * time.Second}

type mockZipkinHandler struct {
	err   error
	mux   sync.Mutex
	spans []*zipkincore.Span
}

func (p *mockZipkinHandler) SubmitZipkinBatch(ctx tchanThrift.Context, spans []*zipkincore.Span) ([]*zipkincore.Response, error) {
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

	deadline := time.Now().Add(2 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Error("never received a span")
			return
		}
		if want, have := 1, len(handler.zipkinSpansHandler.(*mockZipkinHandler).getSpans()); want != have {
			time.Sleep(time.Millisecond)
			continue
		}
		break
	}

	assert.Equal(t, 1, len(handler.zipkinSpansHandler.(*mockZipkinHandler).getSpans()))
}

func TestThriftFormat(t *testing.T) {
	server, handler := initializeTestServer(nil)
	defer server.Close()

	bodyBytes := zipkinSerialize([]*zipkincore.Span{{}})
	statusCode, resBodyStr, err := postBytes(server.URL+`/api/v1/spans`, bodyBytes, createHeader("application/x-thrift"))
	assert.NoError(t, err)
	assert.EqualValues(t, http.StatusAccepted, statusCode)
	assert.EqualValues(t, "", resBodyStr)

	handler.zipkinSpansHandler.(*mockZipkinHandler).err = fmt.Errorf("Bad times ahead")
	statusCode, resBodyStr, err = postBytes(server.URL+`/api/v1/spans`, bodyBytes, createHeader("application/x-thrift"))
	assert.NoError(t, err)
	assert.EqualValues(t, http.StatusInternalServerError, statusCode)
	assert.EqualValues(t, "Cannot submit Zipkin batch: Bad times ahead\n", resBodyStr)
}

func TestJsonFormat(t *testing.T) {
	server, handler := initializeTestServer(nil)
	defer server.Close()

	endpJSON := createEndpoint("foo", "127.0.0.1", "2001:db8::c001", 66)
	annoJSON := createAnno("cs", 1515, endpJSON)
	binAnnoJSON := createBinAnno("http.status_code", "200", endpJSON)
	spanJSON := createSpan("bar", "1234567891234565", "1234567891234567", "1234567891234568", 156, 15145, false,
		annoJSON, binAnnoJSON)
	statusCode, resBodyStr, err := postBytes(server.URL+`/api/v1/spans`, []byte(spanJSON), createHeader("application/json"))
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
		{payload: spanJSON, expected: "Cannot submit Zipkin batch: Bad times ahead\n", statusCode: http.StatusInternalServerError},
		{payload: createSpan("bar", "", "1", "1", 156, 15145, false, annoJSON, binAnnoJSON),
			expected: "Unable to process request body: id is not an unsigned long\n", statusCode: http.StatusBadRequest},
		{payload: createSpan("bar", "ZTA", "1", "1", 156, 15145, false, "", ""),
			expected: "Unable to process request body: id is not an unsigned long\n", statusCode: http.StatusBadRequest},
		{payload: createSpan("bar", "1", "", "1", 156, 15145, false, "", createAnno("cs", 1, endpErrJSON)),
			expected: "Unable to process request body: wrong ipv4\n", statusCode: http.StatusBadRequest},
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
	bodyBytes := zipkinSerialize([]*zipkincore.Span{{}})
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
	assert.EqualValues(t, "Unable to process request body: *zipkincore.Span field 0 read error: EOF\n", resBodyStr)
}

func TestDeserializeWithBadListStart(t *testing.T) {
	spanBytes := zipkinSerialize([]*zipkincore.Span{{}})
	_, err := deserializeThrift(append([]byte{0, 255, 255}, spanBytes...))
	assert.Error(t, err)
}

func TestCannotReadBodyFromRequest(t *testing.T) {
	handler := NewAPIHandler(&mockZipkinHandler{})
	req, err := http.NewRequest(http.MethodPost, "whatever", &errReader{})
	assert.NoError(t, err)
	rw := dummyResponseWriter{}
	handler.saveSpans(&rw, req)
	assert.EqualValues(t, http.StatusInternalServerError, rw.myStatusCode)
	assert.EqualValues(t, "Unable to process request body: Simulated error reading body\n", rw.myBody)
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

func zipkinSerialize(spans []*zipkincore.Span) []byte {
	t := thrift.NewTMemoryBuffer()
	p := thrift.NewTBinaryProtocolTransport(t)
	p.WriteListBegin(thrift.STRUCT, len(spans))
	for _, s := range spans {
		s.Write(p)
	}
	p.WriteListEnd()
	return t.Buffer.Bytes()
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
