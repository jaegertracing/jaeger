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
	"testing"
	"time"

	"github.com/apache/thrift/lib/go/thrift"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	tchanThrift "github.com/uber/tchannel-go/thrift"

	"github.com/uber/jaeger/thrift-gen/jaeger"
	tJaeger "github.com/uber/jaeger/thrift-gen/jaeger"
)

var httpClient = &http.Client{Timeout: 2 * time.Second}

type possiblyErroringJaegerBatchesHandler struct {
	err error
}

func (p *possiblyErroringJaegerBatchesHandler) SubmitBatches(ctx tchanThrift.Context, batches []*jaeger.Batch) ([]*jaeger.BatchSubmitResponse, error) {
	return nil, p.err
}

func initializeTestServer(err error) (*httptest.Server, *APIHandler) {
	r := mux.NewRouter()
	handler := NewAPIHandler(&possiblyErroringJaegerBatchesHandler{err})
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
	spans := []*jaeger.Span{}
	spans = append(spans, span)
	batch := jaeger.Batch{Process: process, Spans: spans}
	tser := thrift.NewTSerializer()
	req := tJaeger.CollectorSubmitBatchesArgs{Batches: []*jaeger.Batch{&batch}}
	someBytes, err := tser.Write(&req)
	assert.NoError(t, err)
	assert.NotEmpty(t, someBytes)
	server, handler := initializeTestServer(nil)
	defer server.Close()

	statusCode, resBodyStr, err := postJSON(server.URL+`/api/traces?format=jaeger.thrift`, someBytes)
	assert.NoError(t, err)
	assert.EqualValues(t, http.StatusOK, statusCode)
	assert.EqualValues(t, "", resBodyStr)

	handler.jaegerBatchesHandler.(*possiblyErroringJaegerBatchesHandler).err = fmt.Errorf("Bad times ahead")
	statusCode, resBodyStr, err = postJSON(server.URL+`/api/traces?format=jaeger.thrift`, someBytes)
	assert.NoError(t, err)
	assert.EqualValues(t, http.StatusBadRequest, statusCode)
	assert.EqualValues(t, "Cannot submit Jaeger batch due to error: Bad times ahead\n", resBodyStr)
}

func TestJaegerFormatBadBody(t *testing.T) {
	server, _ := initializeTestServer(nil)
	defer server.Close()
	bodyBytes := []byte("not good")
	statusCode, resBodyStr, err := postJSON(server.URL+`/api/traces?format=jaeger.thrift`, bodyBytes)
	assert.NoError(t, err)
	assert.EqualValues(t, http.StatusBadRequest, statusCode)
	assert.EqualValues(t, "Cannot deserialize body due to error: *jaeger.CollectorSubmitBatchesArgs field 25711 read error: unexpected EOF\n", resBodyStr)
}

func TestWrongFormat(t *testing.T) {
	server, _ := initializeTestServer(nil)
	defer server.Close()
	statusCode, resBodyStr, err := postJSON(server.URL+`/api/traces?format=nosoupforyou`, []byte{})
	assert.NoError(t, err)
	assert.EqualValues(t, http.StatusBadRequest, statusCode)
	assert.EqualValues(t, "Unsupported format type: nosoupforyou\n", resBodyStr)
}

func TestCannotReadBodyFromRequest(t *testing.T) {
	handler := NewAPIHandler(&possiblyErroringJaegerBatchesHandler{nil})
	req, err := http.NewRequest(http.MethodPost, `/api/traces?format=jaeger.thrift`, &errReader{})
	assert.NoError(t, err)
	rw := dummyResponseWriter{}
	handler.saveSpan(&rw, req)
	assert.EqualValues(t, http.StatusBadRequest, rw.myStatusCode)
	assert.EqualValues(t, "Unable to read from body due to error: Simulated error reading body\n", rw.myBody)
}

type errReader struct {}

func (e *errReader) Read(p []byte) (int, error) {
	return 0, fmt.Errorf("Simulated error reading body")
}

type dummyResponseWriter struct{
	myBody string
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

func postJSON(urlStr string, bodyBytes []byte) (int, string, error) {
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
