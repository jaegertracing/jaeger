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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- Mock implementations ---

type mockMCPClient struct {
	topologyResponse     string
	criticalPathResponse string
	topologyErr          error
	criticalPathErr      error
}

func (m *mockMCPClient) GetTraceTopology(_ context.Context, _ string) (string, error) {
	return m.topologyResponse, m.topologyErr
}

func (m *mockMCPClient) GetCriticalPath(_ context.Context, _ string) (string, error) {
	return m.criticalPathResponse, m.criticalPathErr
}

type mockLLMClient struct {
	response string
	err      error
}

func (m *mockLLMClient) AnalyzeTrace(_ context.Context, _ string) (string, error) {
	return m.response, m.err
}

// --- Helper: create a minimal test server with AI wired up ---

func initializeAITestServer(t *testing.T, mcpClient MCPClient, llmClient LLMClient) *httptest.Server {
	t.Helper()
	aiSvc := NewAIService(mcpClient, llmClient)
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
		&mockMCPClient{
			topologyResponse:     "A -> B -> C",
			criticalPathResponse: "B (100ms) -> C (200ms)",
		},
		&mockLLMClient{response: "Service C is the bottleneck."},
	)

	answer, err := svc.AnalyzeTrace(context.Background(), "abc123", "Why is this trace slow?")
	require.NoError(t, err)
	assert.Equal(t, "Service C is the bottleneck.", answer)
}

func TestAIServiceAnalyzeTraceMCPTopologyError(t *testing.T) {
	svc := NewAIService(
		&mockMCPClient{topologyErr: errors.New("connection refused")},
		&mockLLMClient{response: "unused"},
	)

	_, err := svc.AnalyzeTrace(context.Background(), "abc123", "question")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get trace topology")
}

func TestAIServiceAnalyzeTraceMCPCriticalPathError(t *testing.T) {
	svc := NewAIService(
		&mockMCPClient{
			topologyResponse: "A -> B",
			criticalPathErr:  errors.New("timeout"),
		},
		&mockLLMClient{response: "unused"},
	)

	_, err := svc.AnalyzeTrace(context.Background(), "abc123", "question")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get critical path")
}

func TestAIServiceAnalyzeTraceLLMError(t *testing.T) {
	svc := NewAIService(
		&mockMCPClient{
			topologyResponse:     "A -> B",
			criticalPathResponse: "B (50ms)",
		},
		&mockLLMClient{err: errors.New("model not loaded")},
	)

	_, err := svc.AnalyzeTrace(context.Background(), "abc123", "question")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "LLM analysis failed")
}

// --- HTTP handler tests ---

func TestAnalyzeTraceAISuccess(t *testing.T) {
	ts := initializeAITestServer(t,
		&mockMCPClient{
			topologyResponse:     "frontend -> backend -> db",
			criticalPathResponse: "backend (80ms) -> db (150ms)",
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
	ts := initializeAITestServer(t, &StubMCPClient{}, &StubLLMClient{})

	resp, err := http.Post(ts.URL+"/api/ai/analyze", "application/json", bytes.NewBufferString("not json"))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestAnalyzeTraceAIMissingTraceID(t *testing.T) {
	ts := initializeAITestServer(t, &StubMCPClient{}, &StubLLMClient{})

	body := `{"question": "Why is this slow?"}`
	resp, err := http.Post(ts.URL+"/api/ai/analyze", "application/json", bytes.NewBufferString(body))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestAnalyzeTraceAIMissingQuestion(t *testing.T) {
	ts := initializeAITestServer(t, &StubMCPClient{}, &StubLLMClient{})

	body := `{"traceID": "abc123"}`
	resp, err := http.Post(ts.URL+"/api/ai/analyze", "application/json", bytes.NewBufferString(body))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestAnalyzeTraceAIInternalError(t *testing.T) {
	ts := initializeAITestServer(t,
		&mockMCPClient{topologyErr: errors.New("MCP server unreachable")},
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
	ts := initializeAITestServer(t, &StubMCPClient{}, &StubLLMClient{})

	resp, err := http.Get(ts.URL + "/api/ai/analyze")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
}

// --- Stub tests ---

func TestStubMCPClient(t *testing.T) {
	client := &StubMCPClient{}

	topology, err := client.GetTraceTopology(context.Background(), "test-trace-1")
	require.NoError(t, err)
	assert.Contains(t, topology, "test-trace-1")

	critPath, err := client.GetCriticalPath(context.Background(), "test-trace-1")
	require.NoError(t, err)
	assert.Contains(t, critPath, "test-trace-1")
}

func TestStubLLMClient(t *testing.T) {
	client := &StubLLMClient{}

	answer, err := client.AnalyzeTrace(context.Background(), "some prompt")
	require.NoError(t, err)
	assert.NotEmpty(t, answer)
}

func TestBuildAnalysisPrompt(t *testing.T) {
	prompt := buildAnalysisPrompt("trace-123", "A -> B", "B (100ms)", "Why slow?")
	assert.Contains(t, prompt, "trace-123")
	assert.Contains(t, prompt, "A -> B")
	assert.Contains(t, prompt, "B (100ms)")
	assert.Contains(t, prompt, "Why slow?")
}
