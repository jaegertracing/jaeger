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
	jaegerClient "github.com/uber/jaeger-client-go"
	"github.com/uber/jaeger-client-go/transport"

	"github.com/jaegertracing/jaeger/pkg/clientcfg/clientcfghttp"
	"github.com/jaegertracing/jaeger/thrift-gen/jaeger"
)

var httpClient = &http.Client{Timeout: 2 * time.Second}

type mockJaegerHandler struct {
	err     error
	mux     sync.Mutex
	batches []*jaeger.Batch
}

func (p *mockJaegerHandler) SubmitBatches(batches []*jaeger.Batch, _ SubmitBatchOptions) ([]*jaeger.BatchSubmitResponse, error) {
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

func initializeTestServer(err error) (*httptest.Server, *APIHandler) {
	r := mux.NewRouter()
	handler := NewAPIHandler(&mockJaegerHandler{err: err}, clientcfghttp.HTTPHandler{})
	handler.RegisterRoutes(r)
	return httptest.NewServer(r), handler
}

func TestThriftFormat(t *testing.T) {
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

	statusCode, resBodyStr, err := postBytes("application/x-thrift", server.URL+`/api/traces`, someBytes)
	assert.NoError(t, err)
	assert.EqualValues(t, http.StatusAccepted, statusCode)
	assert.EqualValues(t, "", resBodyStr)

	statusCode, resBodyStr, err = postBytes("application/x-thrift; charset=utf-8", server.URL+`/api/traces`, someBytes)
	assert.NoError(t, err)
	assert.EqualValues(t, http.StatusAccepted, statusCode)
	assert.EqualValues(t, "", resBodyStr)

	handler.jaegerBatchesHandler.(*mockJaegerHandler).err = fmt.Errorf("Bad times ahead")
	statusCode, resBodyStr, err = postBytes("application/vnd.apache.thrift.binary", server.URL+`/api/traces`, someBytes)
	assert.NoError(t, err)
	assert.EqualValues(t, http.StatusInternalServerError, statusCode)
	assert.EqualValues(t, "Cannot submit Jaeger batch: Bad times ahead\n", resBodyStr)
}

func TestViaClient(t *testing.T) {
	server, handler := initializeTestServer(nil)
	defer server.Close()

	sender := transport.NewHTTPTransport(
		server.URL+`/api/traces`,
		transport.HTTPBatchSize(1),
	)

	tracer, closer := jaegerClient.NewTracer(
		"test",
		jaegerClient.NewConstSampler(true),
		jaegerClient.NewRemoteReporter(sender),
	)
	defer closer.Close()

	tracer.StartSpan("root").Finish()

	deadline := time.Now().Add(2 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Error("never received a span")
			return
		}
		if want, have := 1, len(handler.jaegerBatchesHandler.(*mockJaegerHandler).getBatches()); want != have {
			time.Sleep(time.Millisecond)
			continue
		}
		break
	}

	assert.Equal(t, 1, len(handler.jaegerBatchesHandler.(*mockJaegerHandler).getBatches()))
}

func TestBadBody(t *testing.T) {
	server, _ := initializeTestServer(nil)
	defer server.Close()
	bodyBytes := []byte("not good")
	statusCode, resBodyStr, err := postBytes("application/x-thrift", server.URL+`/api/traces`, bodyBytes)
	assert.NoError(t, err)
	assert.EqualValues(t, http.StatusBadRequest, statusCode)
	assert.EqualValues(t, "Unable to process request body: *jaeger.Batch field 25711 read error: unexpected EOF\n", resBodyStr)
}

func TestWrongFormat(t *testing.T) {
	server, _ := initializeTestServer(nil)
	defer server.Close()
	statusCode, resBodyStr, err := postBytes("nosoupforyou", server.URL+`/api/traces`, []byte{})
	assert.NoError(t, err)
	assert.EqualValues(t, http.StatusBadRequest, statusCode)
	assert.EqualValues(t, "Unsupported content type: nosoupforyou\n", resBodyStr)
}

func TestMalformedFormat(t *testing.T) {
	server, _ := initializeTestServer(nil)
	defer server.Close()
	statusCode, resBodyStr, err := postBytes("application/json; =iammalformed", server.URL+`/api/traces`, []byte{})
	assert.NoError(t, err)
	assert.EqualValues(t, http.StatusBadRequest, statusCode)
	assert.EqualValues(t, "Cannot parse content type: mime: invalid media parameter\n", resBodyStr)
}

func TestCannotReadBodyFromRequest(t *testing.T) {
	handler := NewAPIHandler(&mockJaegerHandler{}, clientcfghttp.HTTPHandler{})
	req, err := http.NewRequest(http.MethodPost, "whatever", &errReader{})
	assert.NoError(t, err)
	rw := dummyResponseWriter{}
	handler.SaveSpan(&rw, req)
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

func postBytes(contentType, urlStr string, bodyBytes []byte) (int, string, error) {
	req, err := http.NewRequest(http.MethodPost, urlStr, bytes.NewBuffer([]byte(bodyBytes)))
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Content-Type", contentType)
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
