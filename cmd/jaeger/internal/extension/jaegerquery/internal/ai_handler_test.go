// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- Mock implementations ---

type mockTraceReader struct {
	trace *model.Trace
	err   error
}

func (m *mockTraceReader) GetTrace(_ context.Context, _ string) (*model.Trace, error) {
	return m.trace, m.err
}

type mockLLMClient struct {
	response string
	err      error
}

func (m *mockLLMClient) ExtractParameters(_ context.Context, _ string) (string, error) {
	return m.response, m.err
}

func (m *mockLLMClient) SummarizeTrace(_ context.Context, _ string) (string, error) {
	return m.response, m.err
}

// --- Helper: create a minimal test server with AI wired up ---

func initializeAITestServer(t *testing.T, traceReader TraceReader, llmClient LLMClient) *httptest.Server {
	t.Helper()
	aiSvc := NewAIService(traceReader, llmClient)
	apiHandler := NewAPIHandler(nil,
		HandlerOptions.Logger(zap.NewNop()),
		HandlerOptions.AIService(aiSvc),
	)
	mux := http.NewServeMux()
	apiHandler.RegisterRoutes(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

// --- AIService unit tests ---

func TestAIServiceAnalyzeTrace(t *testing.T) {
	svc := NewAIService(
		&mockTraceReader{
			trace: &model.Trace{Spans: []*model.Span{{Tags: []model.KeyValue{{Key: "error", VType: model.ValueType_BOOL, VBool: true}}}}},
		},
		&mockLLMClient{response: "Service C is the bottleneck."},
	)

	answer, err := svc.AnalyzeTrace(context.Background(), "abc123", "Why is this trace slow?")
	require.NoError(t, err)
	assert.Equal(t, "Service C is the bottleneck.", answer)
}

func TestAIServiceAnalyzeTraceTraceReaderError(t *testing.T) {
	svc := NewAIService(
		&mockTraceReader{err: errors.New("connection refused")},
		&mockLLMClient{response: "unused"},
	)

	_, err := svc.AnalyzeTrace(context.Background(), "abc123", "question")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to fetch trace")
}

func TestAIServiceAnalyzeTraceLLMError(t *testing.T) {
	svc := NewAIService(
		&mockTraceReader{
			trace: &model.Trace{Spans: []*model.Span{{Tags: []model.KeyValue{{Key: "error", VType: model.ValueType_BOOL, VBool: true}}}}},
		},
		&mockLLMClient{err: errors.New("model not loaded")},
	)

	_, err := svc.AnalyzeTrace(context.Background(), "abc123", "question")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "LLM trace summarization failed")
}

// --- HTTP handler tests ---

func TestAnalyzeTraceAISuccess(t *testing.T) {
	ts := initializeAITestServer(t,
		&mockTraceReader{
			trace: &model.Trace{Spans: []*model.Span{{Tags: []model.KeyValue{{Key: "error", VType: model.ValueType_BOOL, VBool: true}}}}},
		},
		&mockLLMClient{
			response: "The database query in the db service is the primary bottleneck.",
		},
	)

	body := `{"traceID": "abc123def456", "question": "Why is this trace slow?"}`
	resp, err := http.Post(ts.URL+"/api/ai/analyze", "application/json", bytes.NewBufferString(body))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result structuredResponse
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Total)

	// Data comes back as map[string]any from JSON decoding.
	data, ok := result.Data.(map[string]any)
	require.True(t, ok, "expected Data to be a map")
	assert.Equal(t, "abc123def456", data["traceID"])
	assert.Contains(t, data["answer"], "database query")
}

func TestAnalyzeTraceAIInvalidJSON(t *testing.T) {
	ts := initializeAITestServer(t, &StubTraceReader{}, &StubLLMClient{})

	resp, err := http.Post(ts.URL+"/api/ai/analyze", "application/json", bytes.NewBufferString("not json"))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestAnalyzeTraceAIMissingTraceID(t *testing.T) {
	ts := initializeAITestServer(t, &StubTraceReader{}, &StubLLMClient{})

	body := `{"question": "Why is this slow?"}`
	resp, err := http.Post(ts.URL+"/api/ai/analyze", "application/json", bytes.NewBufferString(body))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestAnalyzeTraceAIMissingQuestion(t *testing.T) {
	ts := initializeAITestServer(t, &StubTraceReader{}, &StubLLMClient{})

	body := `{"traceID": "abc123"}`
	resp, err := http.Post(ts.URL+"/api/ai/analyze", "application/json", bytes.NewBufferString(body))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestAnalyzeTraceAIInternalError(t *testing.T) {
	ts := initializeAITestServer(t,
		&mockTraceReader{err: errors.New("Trace DB unreachable")},
		&mockLLMClient{response: "unused"},
	)

	body := `{"traceID": "abc123", "question": "What happened?"}`
	resp, err := http.Post(ts.URL+"/api/ai/analyze", "application/json", bytes.NewBufferString(body))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

func TestAnalyzeTraceAINoAIService(t *testing.T) {
	apiHandler := NewAPIHandler(nil, HandlerOptions.Logger(zap.NewNop()))
	mux := http.NewServeMux()
	apiHandler.RegisterRoutes(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	body := `{"traceID": "abc123", "question": "Why slow?"}`
	resp, err := http.Post(ts.URL+"/api/ai/analyze", "application/json", bytes.NewBufferString(body))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotImplemented, resp.StatusCode)
}

func TestAnalyzeTraceAIMethodNotAllowed(t *testing.T) {
	ts := initializeAITestServer(t, &StubTraceReader{}, &StubLLMClient{})

	resp, err := http.Get(ts.URL + "/api/ai/analyze")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
}

// --- Stub tests ---

func TestStubTraceReader(t *testing.T) {
	client := &StubTraceReader{}

	trace, err := client.GetTrace(context.Background(), "test-trace-1")
	require.NoError(t, err)
	assert.NotNil(t, trace)
}

func TestStubLLMClient(t *testing.T) {
	client := &StubLLMClient{}

	answer, err := client.SummarizeTrace(context.Background(), "some prompt")
	require.NoError(t, err)
	assert.NotEmpty(t, answer)
}

func TestBuildContextualPrompt(t *testing.T) {
	prompt := buildContextualPrompt("pruned trace 123", "Why slow?")
	assert.Contains(t, prompt, "pruned trace 123")
	assert.Contains(t, prompt, "Why slow?")
}

// --- /api/ai/search handler tests ---

func TestGenerateSearchParamsAISuccess(t *testing.T) {
	ts := initializeAITestServer(t,
		&StubTraceReader{},
		&mockLLMClient{
			response: `{"service": "payment-service", "operation": "checkout", "tags": {"error": "true"}}`,
		},
	)

	body := `{"question": "Find slow checkout requests in payment-service"}`
	resp, err := http.Post(ts.URL+"/api/ai/search", "application/json", bytes.NewBufferString(body))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result structuredResponse
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Total)

	data, ok := result.Data.(map[string]any)
	require.True(t, ok, "expected Data to be a map")
	assert.Equal(t, "Find slow checkout requests in payment-service", data["originalQuestion"])
	params, ok := data["parameters"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "payment-service", params["service"])
}

func TestGenerateSearchParamsAIInvalidJSON(t *testing.T) {
	ts := initializeAITestServer(t, &StubTraceReader{}, &StubLLMClient{})

	resp, err := http.Post(ts.URL+"/api/ai/search", "application/json", bytes.NewBufferString("not json"))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestGenerateSearchParamsAIMissingQuestion(t *testing.T) {
	ts := initializeAITestServer(t, &StubTraceReader{}, &StubLLMClient{})

	body := `{}`
	resp, err := http.Post(ts.URL+"/api/ai/search", "application/json", bytes.NewBufferString(body))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestGenerateSearchParamsAINoAIService(t *testing.T) {
	apiHandler := NewAPIHandler(nil, HandlerOptions.Logger(zap.NewNop()))
	mux := http.NewServeMux()
	apiHandler.RegisterRoutes(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	body := `{"question": "Find errors"}`
	resp, err := http.Post(ts.URL+"/api/ai/search", "application/json", bytes.NewBufferString(body))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotImplemented, resp.StatusCode)
}

func TestGenerateSearchParamsAILLMError(t *testing.T) {
	ts := initializeAITestServer(t,
		&StubTraceReader{},
		&mockLLMClient{err: errors.New("model not loaded")},
	)

	body := `{"question": "Find errors"}`
	resp, err := http.Post(ts.URL+"/api/ai/search", "application/json", bytes.NewBufferString(body))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}
