// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/acp-go-sdk"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/version"
)

const testWaitForTurnTimeout = 100 * time.Millisecond

type mockACPAgent struct {
	mu sync.Mutex

	initializeReq *acp.InitializeRequest
	newSessionReq *acp.NewSessionRequest
	promptReq     *acp.PromptRequest

	initializeErr error
	newSessionErr error
	promptErr     error
}

func (*mockACPAgent) Authenticate(context.Context, acp.AuthenticateRequest) (acp.AuthenticateResponse, error) {
	return acp.AuthenticateResponse{}, nil
}

func (a *mockACPAgent) Initialize(_ context.Context, params acp.InitializeRequest) (acp.InitializeResponse, error) {
	if a.initializeErr != nil {
		return acp.InitializeResponse{}, a.initializeErr
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	cp := params
	a.initializeReq = &cp
	return acp.InitializeResponse{
		ProtocolVersion:   params.ProtocolVersion,
		AgentCapabilities: acp.AgentCapabilities{},
		AuthMethods:       []acp.AuthMethod{},
	}, nil
}

func (*mockACPAgent) Cancel(context.Context, acp.CancelNotification) error {
	return nil
}

func (a *mockACPAgent) NewSession(_ context.Context, params acp.NewSessionRequest) (acp.NewSessionResponse, error) {
	if a.newSessionErr != nil {
		return acp.NewSessionResponse{}, a.newSessionErr
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	cp := params
	a.newSessionReq = &cp
	return acp.NewSessionResponse{SessionId: "sess-test"}, nil
}

func (a *mockACPAgent) Prompt(ctx context.Context, params acp.PromptRequest) (acp.PromptResponse, error) {
	if a.promptErr != nil {
		return acp.PromptResponse{}, a.promptErr
	}
	a.mu.Lock()
	cp := params
	a.promptReq = &cp
	a.mu.Unlock()
	_ = ctx

	return acp.PromptResponse{StopReason: acp.StopReasonEndTurn}, nil
}

func (*mockACPAgent) SetSessionMode(context.Context, acp.SetSessionModeRequest) (acp.SetSessionModeResponse, error) {
	return acp.SetSessionModeResponse{}, nil
}

func (a *mockACPAgent) snapshot() (*acp.InitializeRequest, *acp.NewSessionRequest, *acp.PromptRequest) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.initializeReq, a.newSessionReq, a.promptReq
}

func startMockACPWebSocketServer(t *testing.T, agent *mockACPAgent) (string, func()) {
	t.Helper()

	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		adapter := NewWsAdapter(conn)
		asc := acp.NewAgentSideConnection(agent, adapter, adapter)

		<-asc.Done()
	}))

	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		server.Config.Shutdown(ctx)
		server.Close()
	}

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	return wsURL, cleanup
}

func TestChatHandlerSendsACPProtocolRequests(t *testing.T) {
	agent := &mockACPAgent{}
	wsURL, cleanup := startMockACPWebSocketServer(t, agent)
	defer cleanup()

	handler := NewChatHandler(zap.NewNop(), wsURL, testWaitForTurnTimeout, 0)

	reqBody, err := json.Marshal(ChatRequest{Prompt: "trace for service checkout"})
	require.NoError(t, err, "failed to marshal request")

	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(reqBody))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, "unexpected status code, body=%q", rr.Body.String())

	initReq, sessionReq, promptReq := agent.snapshot()
	require.NotNil(t, initReq, "expected initialize request to be captured")
	require.NotNil(t, sessionReq, "expected new session request to be captured")
	require.NotNil(t, promptReq, "expected prompt request to be captured")

	require.EqualValues(t, acp.ProtocolVersionNumber, initReq.ProtocolVersion, "initialize protocol version mismatch")
	require.False(t, initReq.ClientCapabilities.Fs.ReadTextFile || initReq.ClientCapabilities.Fs.WriteTextFile || initReq.ClientCapabilities.Terminal, "unexpected client capabilities in initialize: %+v", initReq.ClientCapabilities)
	require.NotNil(t, initReq.ClientInfo, "client info should not be nil")
	require.Equal(t, "jaeger-ai-gateway", initReq.ClientInfo.Name, "client name mismatch")
	require.Equal(t, version.Get().GitVersion, initReq.ClientInfo.Version, "client version mismatch")

	require.Equal(t, "/", sessionReq.Cwd, "session/new cwd mismatch")
	require.Empty(t, sessionReq.McpServers, "expected no MCP servers in session/new")

	require.EqualValues(t, "sess-test", promptReq.SessionId, "prompt sessionId mismatch")
	require.Len(t, promptReq.Prompt, 1, "prompt content length mismatch")
	require.NotNil(t, promptReq.Prompt[0].Text, "prompt text should not be nil")
	require.Equal(t, "trace for service checkout", promptReq.Prompt[0].Text.Text, "prompt text mismatch")

	require.Equal(t, "text/plain; charset=utf-8", rr.Header().Get("Content-Type"), "content type mismatch")
	require.Equal(t, "no-cache", rr.Header().Get("Cache-Control"), "cache-control mismatch")
	require.Equal(t, "keep-alive", rr.Header().Get("Connection"), "connection header mismatch")
}

type noFlusherResponseWriter struct {
	header http.Header
	body   bytes.Buffer
	status int
}

type failingFlusherResponseWriter struct {
	header http.Header
	status int
}

func (w *noFlusherResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *noFlusherResponseWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(p)
}

func (w *noFlusherResponseWriter) WriteHeader(statusCode int) {
	w.status = statusCode
}

func (w *failingFlusherResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *failingFlusherResponseWriter) Write([]byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return 0, errors.New("forced write failure")
}

func (w *failingFlusherResponseWriter) WriteHeader(statusCode int) {
	w.status = statusCode
}

func (*failingFlusherResponseWriter) Flush() {}

func TestNewChatHandlerAppliesDefaults(t *testing.T) {
	h := NewChatHandler(zap.NewNop(), "ws://localhost:1", 0, 0)
	require.Equal(t, defaultWaitForTurnTimeout, h.waitForTurnTimeout, "expected default waitForTurnTimeout")
	require.Equal(t, defaultMaxRequestBodySize, h.maxRequestBodySize, "expected default maxRequestBodySize")

	h2 := NewChatHandler(zap.NewNop(), "ws://localhost:1", 5*time.Millisecond, 512)
	require.Equal(t, 5*time.Millisecond, h2.waitForTurnTimeout, "expected custom waitForTurnTimeout")
	require.Equal(t, int64(512), h2.maxRequestBodySize, "expected custom maxRequestBodySize")
}

func TestChatHandlerMethodNotAllowed(t *testing.T) {
	handler := NewChatHandler(zap.NewNop(), "ws://127.0.0.1:1", testWaitForTurnTimeout, 0)
	req := httptest.NewRequest(http.MethodGet, "/api/ai/chat", http.NoBody)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusMethodNotAllowed, rr.Code, "unexpected status code")
}

func TestChatHandlerBadRequest(t *testing.T) {
	handler := NewChatHandler(zap.NewNop(), "ws://127.0.0.1:1", testWaitForTurnTimeout, 0)
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", strings.NewReader("{"))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code, "unexpected status code")
}

func TestChatHandlerStreamingUnsupported(t *testing.T) {
	handler := NewChatHandler(zap.NewNop(), "ws://127.0.0.1:1", testWaitForTurnTimeout, 0)
	body, err := json.Marshal(ChatRequest{Prompt: "hello"})
	require.NoError(t, err, "failed to marshal request")
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(body))
	w := &noFlusherResponseWriter{}

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.status, "unexpected status code")
}

func TestChatHandlerDialFailure(t *testing.T) {
	handler := NewChatHandler(zap.NewNop(), "ws://127.0.0.1:1", testWaitForTurnTimeout, 0)
	body, err := json.Marshal(ChatRequest{Prompt: "hello"})
	require.NoError(t, err, "failed to marshal request")
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadGateway, rr.Code, "unexpected status code")
}

func TestChatHandlerInitializeError(t *testing.T) {
	agent := &mockACPAgent{initializeErr: errors.New("initialize failed")}
	wsURL, cleanup := startMockACPWebSocketServer(t, agent)
	defer cleanup()

	handler := NewChatHandler(zap.NewNop(), wsURL, testWaitForTurnTimeout, 0)
	body, err := json.Marshal(ChatRequest{Prompt: "hello"})
	require.NoError(t, err, "failed to marshal request")
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadGateway, rr.Code, "unexpected status code, body=%q", rr.Body.String())
	require.Contains(t, rr.Body.String(), "Error initializing agent", "expected initialize error message")
}

func TestChatHandlerNewSessionError(t *testing.T) {
	agent := &mockACPAgent{newSessionErr: errors.New("new session failed")}
	wsURL, cleanup := startMockACPWebSocketServer(t, agent)
	defer cleanup()

	handler := NewChatHandler(zap.NewNop(), wsURL, testWaitForTurnTimeout, 0)
	body, err := json.Marshal(ChatRequest{Prompt: "hello"})
	require.NoError(t, err, "failed to marshal request")
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadGateway, rr.Code, "unexpected status code, body=%q", rr.Body.String())
	require.Contains(t, rr.Body.String(), "Error creating session", "expected session error message")
}

func TestChatHandlerPromptError(t *testing.T) {
	agent := &mockACPAgent{promptErr: errors.New("prompt failed")}
	wsURL, cleanup := startMockACPWebSocketServer(t, agent)
	defer cleanup()

	handler := NewChatHandler(zap.NewNop(), wsURL, testWaitForTurnTimeout, 0)
	body, err := json.Marshal(ChatRequest{Prompt: "hello"})
	require.NoError(t, err, "failed to marshal request")
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadGateway, rr.Code, "unexpected status code, body=%q", rr.Body.String())
	require.Contains(t, rr.Body.String(), "Error starting prompt", "expected prompt error message")
}

func TestChatHandlerErrorWriteFailurePaths(t *testing.T) {
	tests := []struct {
		name  string
		agent *mockACPAgent
	}{
		{name: "initialize error write failure", agent: &mockACPAgent{initializeErr: errors.New("initialize failed")}},
		{name: "new session error write failure", agent: &mockACPAgent{newSessionErr: errors.New("new session failed")}},
		{name: "prompt error write failure", agent: &mockACPAgent{promptErr: errors.New("prompt failed")}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			wsURL, cleanup := startMockACPWebSocketServer(t, tc.agent)
			defer cleanup()

			handler := NewChatHandler(zap.NewNop(), wsURL, testWaitForTurnTimeout, 0)
			body, err := json.Marshal(ChatRequest{Prompt: "hello"})
			require.NoError(t, err, "failed to marshal request")
			req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(body))
			w := &failingFlusherResponseWriter{}

			handler.ServeHTTP(w, req)

			require.Equal(t, http.StatusBadGateway, w.status, "unexpected status code")
		})
	}
}
