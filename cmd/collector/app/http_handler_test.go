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

package app

import (
	"bytes"
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
	"github.com/uber/jaeger-client-go/transport"
	zipkinTransport "github.com/uber/jaeger-client-go/transport/zipkin"
	tchanThrift "github.com/uber/tchannel-go/thrift"

	"github.com/uber/jaeger/thrift-gen/jaeger"
	"github.com/uber/jaeger/thrift-gen/zipkincore"
)

var httpClient = &http.Client{Timeout: 2 * time.Second}

type mockJaegerHandler struct {
	err     error
	mux     sync.Mutex
	batches []*jaeger.Batch
}

func (p *mockJaegerHandler) SubmitBatches(ctx tchanThrift.Context, batches []*jaeger.Batch) ([]*jaeger.BatchSubmitResponse, error) {
	p.mux.Lock()
	defer p.mux.Unlock()
	p.batches = append(p.batches, batches...)
	return nil, p.err
}

func (p *mockJaegerHandler) getBatches() []*jaeger.Batch {
	p.mux.Lock()
	defer p.mux.Unlock()
	return p.batches
}

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
	handler := NewAPIHandler(&mockJaegerHandler{err: err}, &mockZipkinHandler{err: err})
	handler.RegisterRoutes(r)
	return httptest.NewServer(r), handler
}

func TestJaegerFormat(t *testing.T) {
	process := &jaeger.Process{
		ServiceName: "serviceName",
	}
	span := &jaeger.Span{
		OperationName: "opName",
	}
	spans := []*jaeger.Span{span}
	batch := jaeger.Batch{Process: process, Spans: spans}
	tser := thrift.NewTSerializer()
	someBytes, err := tser.Write(&batch)
	assert.NoError(t, err)
	assert.NotEmpty(t, someBytes)
	server, handler := initializeTestServer(nil)
	defer server.Close()

	statusCode, resBodyStr, err := postBytes(server.URL+`/api/traces?format=jaeger.thrift`, someBytes)
	assert.NoError(t, err)
	assert.EqualValues(t, http.StatusOK, statusCode)
	assert.EqualValues(t, "", resBodyStr)

	handler.jaegerBatchesHandler.(*mockJaegerHandler).err = fmt.Errorf("Bad times ahead")
	statusCode, resBodyStr, err = postBytes(server.URL+`/api/traces?format=jaeger.thrift`, someBytes)
	assert.NoError(t, err)
	assert.EqualValues(t, http.StatusInternalServerError, statusCode)
	assert.EqualValues(t, "Cannot submit Jaeger batch: Bad times ahead\n", resBodyStr)
}

func TestFormatsViaClient(t *testing.T) {
	server, handler := initializeTestServer(nil)
	defer server.Close()

	jaegerSender := transport.NewHTTPTransport(
		server.URL+`/api/traces?format=jaeger.thrift`,
		transport.HTTPBatchSize(1),
	)
	zipkinSender, err := zipkinTransport.NewHTTPTransport(
		server.URL+`/api/traces?format=zipkin.thrift`,
		zipkinTransport.HTTPBatchSize(1),
	)
	require.NoError(t, err)

	testCases := []struct {
		name     string
		sender   jaegerClient.Transport
		received func() int
	}{
		{
			name:   "jaeger.thrift",
			sender: jaegerSender,
			received: func() int {
				return len(handler.jaegerBatchesHandler.(*mockJaegerHandler).getBatches())
			},
		},
		{
			name:   "zipkin.thrift",
			sender: zipkinSender,
			received: func() int {
				return len(handler.zipkinSpansHandler.(*mockZipkinHandler).getSpans())
			},
		},
	}

	for _, tc := range testCases {
		testCase := tc // capture loop var
		t.Run(testCase.name, func(t *testing.T) {
			tracer, closer := jaegerClient.NewTracer(
				"test",
				jaegerClient.NewConstSampler(true),
				jaegerClient.NewRemoteReporter(testCase.sender),
			)
			defer closer.Close()

			tracer.StartSpan("root").Finish()

			deadline := time.Now().Add(2 * time.Second)
			for {
				if time.Now().After(deadline) {
					t.Error("never received a span")
					return
				}
				if want, have := 1, testCase.received(); want != have {
					time.Sleep(time.Millisecond)
					continue
				}
				break
			}

			assert.Equal(t, 1, testCase.received())
		})
	}
}

func TestJaegerFormatBadBody(t *testing.T) {
	server, _ := initializeTestServer(nil)
	defer server.Close()
	bodyBytes := []byte("not good")
	statusCode, resBodyStr, err := postBytes(server.URL+`/api/traces?format=jaeger.thrift`, bodyBytes)
	assert.NoError(t, err)
	assert.EqualValues(t, http.StatusBadRequest, statusCode)
	assert.EqualValues(t, "Unable to process request body: *jaeger.Batch field 25711 read error: unexpected EOF\n", resBodyStr)
}

func TestWrongFormat(t *testing.T) {
	server, _ := initializeTestServer(nil)
	defer server.Close()
	statusCode, resBodyStr, err := postBytes(server.URL+`/api/traces?format=nosoupforyou`, []byte{})
	assert.NoError(t, err)
	assert.EqualValues(t, http.StatusBadRequest, statusCode)
	assert.EqualValues(t, "Unsupported format type: nosoupforyou\n", resBodyStr)
}

func TestCannotReadBodyFromRequest(t *testing.T) {
	handler := NewAPIHandler(&mockJaegerHandler{}, &mockZipkinHandler{})

	testCases := []struct {
		url string
	}{
		{`/api/traces?format=jaeger.thrift`},
		{`/api/traces?format=zipkin.thrift`},
	}
	for _, testCase := range testCases {
		req, err := http.NewRequest(http.MethodPost, testCase.url, &errReader{})
		assert.NoError(t, err)
		rw := dummyResponseWriter{}
		handler.saveSpan(&rw, req)
		assert.EqualValues(t, http.StatusInternalServerError, rw.myStatusCode)
		assert.EqualValues(t, "Unable to process request body: Simulated error reading body\n", rw.myBody)
	}
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

func postBytes(urlStr string, bodyBytes []byte) (int, string, error) {
	req, err := http.NewRequest(http.MethodPost, urlStr, bytes.NewBuffer([]byte(bodyBytes)))
	if err != nil {
		return 0, "", err
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

func TestZipkinFormat(t *testing.T) {
	span := &zipkincore.Span{}
	spans := []*zipkincore.Span{}
	spans = append(spans, span)
	server, handler := initializeTestServer(nil)
	defer server.Close()

	bodyBytes := zipkinSerialize(spans)
	statusCode, resBodyStr, err := postBytes(server.URL+`/api/traces?format=zipkin.thrift`, bodyBytes)
	assert.NoError(t, err)
	assert.EqualValues(t, http.StatusOK, statusCode)
	assert.EqualValues(t, "", resBodyStr)

	handler.zipkinSpansHandler.(*mockZipkinHandler).err = fmt.Errorf("Bad times ahead")
	statusCode, resBodyStr, err = postBytes(server.URL+`/api/traces?format=zipkin.thrift`, bodyBytes)
	assert.NoError(t, err)
	assert.EqualValues(t, http.StatusInternalServerError, statusCode)
	assert.EqualValues(t, "Cannot submit Zipkin batch: Bad times ahead\n", resBodyStr)
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

func TestZipkinFormatBadBody(t *testing.T) {
	server, _ := initializeTestServer(nil)
	defer server.Close()
	bodyBytes := []byte("not good")
	statusCode, resBodyStr, err := postBytes(server.URL+`/api/traces?format=zipkin.thrift`, bodyBytes)
	assert.NoError(t, err)
	assert.EqualValues(t, http.StatusBadRequest, statusCode)
	assert.EqualValues(t, "Unable to process request body: *zipkincore.Span field 0 read error: EOF\n", resBodyStr)
}

func TestDeserializeZipkinWithBadListStart(t *testing.T) {
	span := &zipkincore.Span{TraceID: 12, Name: "test"}
	spans := []*zipkincore.Span{}
	spans = append(spans, span)
	spanBytes := zipkinSerialize(spans)
	_, err := deserializeZipkin(append([]byte{0, 255, 255}, spanBytes...))
	assert.Error(t, err)
}
