// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"bytes"
	"context"
	"errors"
	"io"
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

	"github.com/jaegertracing/jaeger-idl/thrift-gen/jaeger"
)

var (
	httpClient                      = &http.Client{Timeout: 2 * time.Second}
	_          JaegerBatchesHandler = (*mockJaegerHandler)(nil)
)

type mockJaegerHandler struct {
	err     error
	mux     sync.Mutex
	batches []*jaeger.Batch
}

func (p *mockJaegerHandler) SubmitBatches(_ context.Context, batches []*jaeger.Batch, _ SubmitBatchOptions) ([]*jaeger.BatchSubmitResponse, error) {
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
	handler := NewAPIHandler(&mockJaegerHandler{err: err})
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
	someBytes, err := tser.Write(context.Background(), &batch)
	require.NoError(t, err)
	assert.NotEmpty(t, someBytes)
	server, handler := initializeTestServer(nil)
	defer server.Close()

	statusCode, resBodyStr, err := postBytes("application/x-thrift", server.URL+`/api/traces`, someBytes)
	require.NoError(t, err)
	assert.EqualValues(t, http.StatusAccepted, statusCode)
	assert.EqualValues(t, "", resBodyStr)

	statusCode, resBodyStr, err = postBytes("application/x-thrift; charset=utf-8", server.URL+`/api/traces`, someBytes)
	require.NoError(t, err)
	assert.EqualValues(t, http.StatusAccepted, statusCode)
	assert.EqualValues(t, "", resBodyStr)

	handler.jaegerBatchesHandler.(*mockJaegerHandler).err = errors.New("Bad times ahead")
	statusCode, resBodyStr, err = postBytes("application/vnd.apache.thrift.binary", server.URL+`/api/traces`, someBytes)
	require.NoError(t, err)
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

	assert.Len(t, handler.jaegerBatchesHandler.(*mockJaegerHandler).getBatches(), 1)
}

func TestBadBody(t *testing.T) {
	server, _ := initializeTestServer(nil)
	defer server.Close()
	bodyBytes := []byte("not good")
	statusCode, resBodyStr, err := postBytes("application/x-thrift", server.URL+`/api/traces`, bodyBytes)
	require.NoError(t, err)
	assert.EqualValues(t, http.StatusBadRequest, statusCode)
	assert.EqualValues(t, "Unable to process request body: Unknown data type 110\n", resBodyStr)
}

func TestWrongFormat(t *testing.T) {
	server, _ := initializeTestServer(nil)
	defer server.Close()
	statusCode, resBodyStr, err := postBytes("nosoupforyou", server.URL+`/api/traces`, []byte{})
	require.NoError(t, err)
	assert.EqualValues(t, http.StatusBadRequest, statusCode)
	assert.EqualValues(t, "Unsupported content type: nosoupforyou\n", resBodyStr)
}

func TestMalformedFormat(t *testing.T) {
	server, _ := initializeTestServer(nil)
	defer server.Close()
	statusCode, resBodyStr, err := postBytes("application/json; =iammalformed", server.URL+`/api/traces`, []byte{})
	require.NoError(t, err)
	assert.EqualValues(t, http.StatusBadRequest, statusCode)
	assert.EqualValues(t, "Cannot parse content type: mime: invalid media parameter\n", resBodyStr)
}

func TestCannotReadBodyFromRequest(t *testing.T) {
	handler := NewAPIHandler(&mockJaegerHandler{})
	req, err := http.NewRequest(http.MethodPost, "whatever", &errReader{})
	require.NoError(t, err)
	rw := dummyResponseWriter{}
	handler.SaveSpan(&rw, req)
	assert.EqualValues(t, http.StatusInternalServerError, rw.myStatusCode)
	assert.EqualValues(t, "Unable to process request body: Simulated error reading body\n", rw.myBody)
}

type errReader struct{}

func (*errReader) Read([]byte) (int, error) {
	return 0, errors.New("Simulated error reading body")
}

type dummyResponseWriter struct {
	myBody       string
	myStatusCode int
}

func (*dummyResponseWriter) Header() http.Header {
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

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return 0, "", err
	}
	return res.StatusCode, string(body), nil
}
