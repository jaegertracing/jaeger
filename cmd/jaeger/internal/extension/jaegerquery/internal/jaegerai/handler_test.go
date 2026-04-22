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

type mockACPAgent struct {
	mu sync.Mutex

	initializeReq *acp.InitializeRequest
	newSessionReq *acp.NewSessionRequest
	promptReq     *acp.PromptRequest

	promptStopReason acp.StopReason

	initializeErr error
	newSessionErr error
	promptErr     error

	// promptHook is called during Prompt before returning, allowing tests to
	// send SessionUpdate notifications via asc.
	promptHook func(context.Context, *acp.AgentSideConnection, acp.PromptRequest)

	// asc is set by startMockACPWebSocketServer so Prompt can send SessionUpdate notifications.
	asc *acp.AgentSideConnection
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
	hook := a.promptHook
	conn := a.asc
	a.mu.Unlock()

	if hook != nil && conn != nil {
		hook(ctx, conn, cp)
	}

	reason := a.promptStopReason
	if reason == "" {
		reason = acp.StopReasonEndTurn
	}
	return acp.PromptResponse{StopReason: reason}, nil
}

func (*mockACPAgent) SetSessionConfigOption(context.Context, acp.SetSessionConfigOptionRequest) (acp.SetSessionConfigOptionResponse, error) {
	return acp.SetSessionConfigOptionResponse{}, nil
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

		adapter := NewWsAdapter(conn, zap.NewNop())
		asc := acp.NewAgentSideConnection(agent, adapter, adapter)
		agent.mu.Lock()
		agent.asc = asc
		agent.mu.Unlock()

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

	handler := NewChatHandler(zap.NewNop(), nil, wsURL, 1<<20)

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
	require.Empty(t, rr.Header().Get("Connection"), "Connection is a hop-by-hop header managed by net/http")
}

type failingFlusherResponseWriter struct {
	header http.Header
	status int
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

func TestNewChatHandlerPassesThroughConfig(t *testing.T) {
	store := NewContextualToolsStore()
	h := NewChatHandler(zap.NewNop(), store, "ws://localhost:1", 512)
	require.Equal(t, int64(512), h.maxRequestBodySize, "expected configured maxRequestBodySize")
	require.Equal(t, "ws://localhost:1", h.sidecarWSURL, "expected configured sidecarWSURL")
	require.Same(t, store, h.ctxTools, "expected configured ctxTools store")
}

func TestChatHandlerMethodNotAllowed(t *testing.T) {
	handler := NewChatHandler(zap.NewNop(), nil, "ws://127.0.0.1:1", 1<<20)
	req := httptest.NewRequest(http.MethodGet, "/api/ai/chat", http.NoBody)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusMethodNotAllowed, rr.Code, "unexpected status code")
}

func TestChatHandlerBadRequest(t *testing.T) {
	handler := NewChatHandler(zap.NewNop(), nil, "ws://127.0.0.1:1", 1<<20)
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", strings.NewReader("{"))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code, "unexpected status code")
}

func TestChatHandlerEmptyPrompt(t *testing.T) {
	handler := NewChatHandler(zap.NewNop(), nil, "ws://127.0.0.1:1", 1<<20)
	body, err := json.Marshal(ChatRequest{Prompt: "   "})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code, "unexpected status code")
	require.Contains(t, rr.Body.String(), "prompt is required")
}

func TestChatHandlerRequestBodyTooLarge(t *testing.T) {
	handler := NewChatHandler(zap.NewNop(), nil, "ws://127.0.0.1:1", 10)
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", strings.NewReader(`{"prompt":"this body exceeds the 10 byte limit"}`))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusRequestEntityTooLarge, rr.Code, "unexpected status code")
}

func TestChatHandlerDialFailure(t *testing.T) {
	handler := NewChatHandler(zap.NewNop(), nil, "ws://127.0.0.1:1", 1<<20)
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

	handler := NewChatHandler(zap.NewNop(), nil, wsURL, 1<<20)
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

	handler := NewChatHandler(zap.NewNop(), nil, wsURL, 1<<20)
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

	handler := NewChatHandler(zap.NewNop(), nil, wsURL, 1<<20)
	body, err := json.Marshal(ChatRequest{Prompt: "hello"})
	require.NoError(t, err, "failed to marshal request")
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadGateway, rr.Code, "unexpected status code, body=%q", rr.Body.String())
	require.Contains(t, rr.Body.String(), "Error starting prompt", "expected prompt error message")
}

func TestChatHandlerNonEndTurnStopReason(t *testing.T) {
	agent := &mockACPAgent{promptStopReason: acp.StopReasonMaxTokens}
	wsURL, cleanup := startMockACPWebSocketServer(t, agent)
	defer cleanup()

	handler := NewChatHandler(zap.NewNop(), nil, wsURL, 1<<20)
	body, err := json.Marshal(ChatRequest{Prompt: "hello"})
	require.NoError(t, err, "failed to marshal request")
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, "unexpected status code")
	require.Contains(t, rr.Body.String(), "[stop_reason] max_tokens", "expected stop_reason marker in response")
}

func TestChatHandlerSessionUpdateStreamedBeforePromptReturns(t *testing.T) {
	// The agent sends a SessionUpdate notification during Prompt, just before
	// returning the response. acp-go-sdk guarantees all notifications are
	// processed before Prompt() returns (via notificationWg.Wait), so the
	// streamed text must appear in the response without any grace period.
	agent := &mockACPAgent{
		promptHook: func(ctx context.Context, asc *acp.AgentSideConnection, _ acp.PromptRequest) {
			_ = asc.SessionUpdate(ctx, acp.SessionNotification{
				SessionId: "sess-test",
				Update:    acp.UpdateAgentMessageText("streamed-via-notification"),
			})
		},
	}
	wsURL, cleanup := startMockACPWebSocketServer(t, agent)
	defer cleanup()

	handler := NewChatHandler(zap.NewNop(), nil, wsURL, 1<<20)
	body, err := json.Marshal(ChatRequest{Prompt: "hello"})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "streamed-via-notification",
		"SessionUpdate should be flushed before Prompt() returns")
}

func TestChatHandlerPromptErrorWriteFailure(t *testing.T) {
	agent := &mockACPAgent{promptErr: errors.New("prompt failed")}
	wsURL, cleanup := startMockACPWebSocketServer(t, agent)
	defer cleanup()

	handler := NewChatHandler(zap.NewNop(), nil, wsURL, 1<<20)
	body, err := json.Marshal(ChatRequest{Prompt: "hello"})
	require.NoError(t, err, "failed to marshal request")
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(body))
	w := &failingFlusherResponseWriter{}

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadGateway, w.status, "unexpected status code")
}

func TestChatHandlerCleansUpContextualToolsOnTurnEnd(t *testing.T) {
	// With a real ContextualToolsStore supplied, the handler must call
	// DeleteForSession once the turn finishes so the store does not grow
	// unboundedly across requests. We pre-seed the store for the mock
	// agent's session id and assert the entry is gone after ServeHTTP.
	agent := &mockACPAgent{}
	wsURL, cleanup := startMockACPWebSocketServer(t, agent)
	defer cleanup()

	store := NewContextualToolsStore()
	store.SetForSession("sess-test", []json.RawMessage{json.RawMessage(`{"name":"seeded"}`)})
	require.NotNil(t, store.GetContextualToolsForSession("sess-test"),
		"pre-seeded snapshot should be readable before the turn runs")

	handler := NewChatHandler(zap.NewNop(), store, wsURL, 1<<20)
	body, err := json.Marshal(ChatRequest{Prompt: "hello"})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	require.Nil(t, store.GetContextualToolsForSession("sess-test"),
		"handler must drop the snapshot via DeleteForSession once the turn ends")
}
