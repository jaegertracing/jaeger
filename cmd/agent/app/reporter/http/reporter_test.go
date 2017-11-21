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

package http

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics"
	mTestutils "github.com/uber/jaeger-lib/metrics/testutils"
	tchanThrift "github.com/uber/tchannel-go/thrift"
	"go.uber.org/zap"

	cApp "github.com/jaegertracing/jaeger/cmd/collector/app"
	"github.com/jaegertracing/jaeger/cmd/collector/app/zipkin"
	"github.com/jaegertracing/jaeger/thrift-gen/jaeger"
	"github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
)

func TestZipkinHTTPReporterSuccess(t *testing.T) {
	handler := &mockZipkinHandler{}
	server := initializeZipkinTestServer(handler)

	cfg := Builder{}
	// simple cross test: we set two vars and expect it to still work
	cfg.WithCollectorHostPorts([]string{server.URL, server.URL})

	mFactory := metrics.NewLocalFactory(0)
	reporter, err := cfg.CreateReporter(mFactory, zap.NewNop())
	require.NoError(t, err)

	require.NoError(t, submitTestZipkinBatch(reporter))
	checkCounters(t, mFactory, 1, 1, 0, 0, "zipkin")

	require.Equal(t, 1, len(handler.spans))
	assert.Equal(t, "span1", handler.spans[0].Name)
}

func TestNoCollectorHostAndPort(t *testing.T) {
	handler := &mockZipkinHandler{}
	_ = initializeZipkinTestServer(handler)

	cfg := Builder{}
	// simple cross test: we set two vars and expect it to still work
	cfg.WithCollectorHostPorts([]string{})

	mFactory := metrics.NewLocalFactory(0)
	r, err := cfg.CreateReporter(mFactory, zap.NewNop())
	assert.Nil(t, r)
	assert.Error(t, err)
}

func TestZipkinNoExistingCollectorHostAndPort(t *testing.T) {
	handler := &mockZipkinHandler{}
	_ = initializeZipkinTestServer(handler)

	cfg := Builder{}
	// simple cross test: we set two vars and expect it to still work
	cfg.WithCollectorHostPorts([]string{"localhost:112233445566"})

	mFactory := metrics.NewLocalFactory(0)
	reporter, err := cfg.CreateReporter(mFactory, zap.NewNop())
	require.NoError(t, err)

	require.Error(t, submitTestZipkinBatch(reporter))
	checkCounters(t, mFactory, 0, 0, 1, 1, "zipkin")
}

func TestJaegerNoExistingCollectorHostAndPort(t *testing.T) {
	handler := &mockJaegerHandler{}
	_ = initializeJaegerTestServer(handler)

	cfg := Builder{}
	// simple cross test: we set two vars and expect it to still work
	cfg.WithCollectorHostPorts([]string{"localhost:112233445566"})

	mFactory := metrics.NewLocalFactory(0)
	reporter, err := cfg.CreateReporter(mFactory, zap.NewNop())
	require.NoError(t, err)

	require.Error(t, submitTestJaegerBatch(reporter))
	checkCounters(t, mFactory, 0, 0, 1, 1, "jaeger")
}

func TestJaegerHTTPReporterSuccess(t *testing.T) {
	handler := &mockJaegerHandler{}
	server := initializeJaegerTestServer(handler)

	cfg := Builder{}
	cfg.WithCollectorHostPorts([]string{server.URL})

	mFactory := metrics.NewLocalFactory(0)
	reporter, err := cfg.CreateReporter(mFactory, zap.NewNop())
	require.NoError(t, err)

	require.NoError(t, submitTestJaegerBatch(reporter))
	checkCounters(t, mFactory, 1, 1, 0, 0, "jaeger")

	require.Equal(t, 1, len(handler.batches))
	assert.Equal(t, "span1", handler.batches[0].Spans[0].OperationName)
}

func TestReporterSendsUsernameAndPassword(t *testing.T) {
	wg := sync.WaitGroup{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		assert.True(t, ok)
		assert.Equal(t, "jdoe", u)
		assert.Equal(t, "password", p)
		wg.Done()
	}))
	defer ts.Close()

	cfg := Builder{}
	cfg.WithUsername("jdoe").WithPassword("password").WithCollectorHostPorts([]string{ts.URL})
	mFactory := metrics.NewLocalFactory(0)
	reporter, err := cfg.CreateReporter(mFactory, zap.NewNop())
	require.NoError(t, err)

	wg.Add(1)
	require.NoError(t, submitTestJaegerBatch(reporter))
}

func TestReporterSendsAuthToken(t *testing.T) {
	wg := sync.WaitGroup{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		assert.Equal(t, "Bearer the-token", auth)
		wg.Done()
	}))
	defer ts.Close()

	cfg := Builder{}
	cfg.WithAuthToken("the-token").WithCollectorHostPorts([]string{ts.URL})
	mFactory := metrics.NewLocalFactory(0)
	reporter, err := cfg.CreateReporter(mFactory, zap.NewNop())
	require.NoError(t, err)

	wg.Add(1)
	require.NoError(t, submitTestJaegerBatch(reporter))
}

func TestZipkinHTTPReporterFailure(t *testing.T) {
	handler := &mockZipkinHandler{err: errors.New("Some error")}
	server := initializeZipkinTestServer(handler)

	cfg := Builder{}
	cfg.WithCollectorHostPorts([]string{server.URL})

	mFactory := metrics.NewLocalFactory(0)
	reporter, err := cfg.CreateReporter(mFactory, zap.NewNop())
	require.NoError(t, err)

	require.Error(t, submitTestZipkinBatch(reporter))
	checkCounters(t, mFactory, 0, 0, 1, 1, "zipkin")
}

func TestJaegerHTTPReporterFailure(t *testing.T) {
	handler := &mockJaegerHandler{err: errors.New("Some error")}
	server := initializeJaegerTestServer(handler)

	cfg := NewBuilder()
	cfg.WithScheme("http")
	cfg.WithCollectorHostPorts([]string{server.URL})

	mFactory := metrics.NewLocalFactory(0)
	reporter, err := cfg.CreateReporter(mFactory, zap.NewNop())
	require.NoError(t, err)

	require.Error(t, submitTestJaegerBatch(reporter))
	checkCounters(t, mFactory, 0, 0, 1, 1, "jaeger")
}

func submitTestZipkinBatch(reporter *Reporter) error {
	span := zipkincore.NewSpan()
	span.Name = "span1"

	return reporter.EmitZipkinBatch([]*zipkincore.Span{span})
}

func submitTestJaegerBatch(reporter *Reporter) error {
	batch := jaeger.NewBatch()
	batch.Process = jaeger.NewProcess()
	batch.Spans = []*jaeger.Span{{OperationName: "span1"}}

	return reporter.EmitBatch(batch)
}

func checkCounters(t *testing.T, mf *metrics.LocalFactory, batchesSubmitted, spansSubmitted, batchesFailures, spansFailures int, prefix string) {
	batchesCounter := fmt.Sprintf("http-reporter.%s.batches.submitted", prefix)
	batchesFailureCounter := fmt.Sprintf("http-reporter.%s.batches.failures", prefix)
	spansCounter := fmt.Sprintf("http-reporter.%s.spans.submitted", prefix)
	spansFailureCounter := fmt.Sprintf("http-reporter.%s.spans.failures", prefix)

	mTestutils.AssertCounterMetrics(t, mf, []mTestutils.ExpectedMetric{
		{Name: batchesCounter, Value: batchesSubmitted},
		{Name: spansCounter, Value: spansSubmitted},
		{Name: batchesFailureCounter, Value: batchesFailures},
		{Name: spansFailureCounter, Value: spansFailures},
	}...)
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

func initializeZipkinTestServer(zHandler *mockZipkinHandler) *httptest.Server {
	r := mux.NewRouter()
	handler := zipkin.NewAPIHandler(zHandler)
	handler.RegisterRoutes(r)
	return httptest.NewServer(r)
}

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

func initializeJaegerTestServer(jHandler *mockJaegerHandler) *httptest.Server {
	r := mux.NewRouter()
	handler := cApp.NewAPIHandler(jHandler)
	handler.RegisterRoutes(r)
	return httptest.NewServer(r)
}
