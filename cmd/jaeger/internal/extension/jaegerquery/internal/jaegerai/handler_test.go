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

	aguitypes "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
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

// userMessageRequest returns a ChatRequest carrying a single user-role message
// with the supplied text. Tests use it to build minimal AG-UI payloads.
func userMessageRequest(text string) ChatRequest {
	return ChatRequest{
		Messages: []aguitypes.Message{{Role: aguitypes.RoleUser, Content: text}},
	}
}

func TestChatHandlerSendsACPProtocolRequests(t *testing.T) {
	agent := &mockACPAgent{}
	wsURL, cleanup := startMockACPWebSocketServer(t, agent)
	defer cleanup()

	handler := NewChatHandler(zap.NewNop(), nil, wsURL, 1<<20)

	reqBody, err := json.Marshal(ChatRequest{
		ThreadID: "thread-1",
		RunID:    "run-1",
		Messages: []aguitypes.Message{{Role: aguitypes.RoleUser, Content: "trace for service checkout"}},
	})
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

	require.Equal(t, "text/event-stream", rr.Header().Get("Content-Type"), "content type mismatch")
	require.Equal(t, "no-cache", rr.Header().Get("Cache-Control"), "cache-control mismatch")

	body := rr.Body.String()
	require.Contains(t, body, "\"type\":\"RUN_STARTED\"", "expected run started SSE event")
	require.Contains(t, body, "\"runId\":\"run-1\"", "runId should echo the client-provided value")
	require.Contains(t, body, "\"type\":\"RUN_FINISHED\"", "expected run finished SSE event")
	require.Contains(t, body, "\"stopReason\":\"end_turn\"", "expected end_turn stop reason")
}

type failingResponseWriter struct {
	header http.Header
	status int
}

func (w *failingResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *failingResponseWriter) Write([]byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return 0, errors.New("forced write failure")
}

func (w *failingResponseWriter) WriteHeader(statusCode int) {
	w.status = statusCode
}

func TestNewChatHandlerPassesThroughConfig(t *testing.T) {
	h := NewChatHandler(zap.NewNop(), nil, "ws://localhost:1", 512)
	require.Equal(t, int64(512), h.maxRequestBodySize, "expected configured maxRequestBodySize")
	require.Equal(t, "ws://localhost:1", h.sidecarWSURL, "expected configured sidecarWSURL")
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
	body, err := json.Marshal(userMessageRequest("   "))
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code, "unexpected status code")
	require.Contains(t, rr.Body.String(), "prompt is required")
}

func TestChatHandlerRequestBodyTooLarge(t *testing.T) {
	handler := NewChatHandler(zap.NewNop(), nil, "ws://127.0.0.1:1", 10)
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", strings.NewReader(`{"messages":[{"role":"user","content":"this body exceeds the 10 byte limit"}]}`))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusRequestEntityTooLarge, rr.Code, "unexpected status code")
}

func TestChatHandlerDialFailure(t *testing.T) {
	handler := NewChatHandler(zap.NewNop(), nil, "ws://127.0.0.1:1", 1<<20)
	body, err := json.Marshal(userMessageRequest("hello"))
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
	body, err := json.Marshal(userMessageRequest("hello"))
	require.NoError(t, err, "failed to marshal request")
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// SSE headers are committed only just before Prompt(), so errors during
	// Initialize surface as a plain 502 with no streaming contract.
	require.Equal(t, http.StatusBadGateway, rr.Code, "unexpected status code, body=%q", rr.Body.String())
	require.Contains(t, rr.Body.String(), "Error initializing agent", "expected initialize error message")
	require.NotContains(t, rr.Header().Get("Content-Type"), "text/event-stream",
		"initialize errors must not claim to be an SSE stream")
}

func TestChatHandlerNewSessionError(t *testing.T) {
	agent := &mockACPAgent{newSessionErr: errors.New("new session failed")}
	wsURL, cleanup := startMockACPWebSocketServer(t, agent)
	defer cleanup()

	handler := NewChatHandler(zap.NewNop(), nil, wsURL, 1<<20)
	body, err := json.Marshal(userMessageRequest("hello"))
	require.NoError(t, err, "failed to marshal request")
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// Same reasoning as Initialize — no SSE contract yet, use a plain 502.
	require.Equal(t, http.StatusBadGateway, rr.Code, "unexpected status code, body=%q", rr.Body.String())
	require.Contains(t, rr.Body.String(), "Error creating session", "expected session error message")
	require.NotContains(t, rr.Header().Get("Content-Type"), "text/event-stream",
		"new_session errors must not claim to be an SSE stream")
}

func TestChatHandlerPromptError(t *testing.T) {
	agent := &mockACPAgent{promptErr: errors.New("prompt failed")}
	wsURL, cleanup := startMockACPWebSocketServer(t, agent)
	defer cleanup()

	handler := NewChatHandler(zap.NewNop(), nil, wsURL, 1<<20)
	body, err := json.Marshal(userMessageRequest("hello"))
	require.NoError(t, err, "failed to marshal request")
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, "unexpected status code, body=%q", rr.Body.String())
	require.Contains(t, rr.Body.String(), "\"type\":\"RUN_ERROR\"", "expected RUN_ERROR SSE event")
	require.Contains(t, rr.Body.String(), "Error starting prompt", "expected prompt error message")
}

func TestChatHandlerNonEndTurnStopReason(t *testing.T) {
	agent := &mockACPAgent{promptStopReason: acp.StopReasonMaxTokens}
	wsURL, cleanup := startMockACPWebSocketServer(t, agent)
	defer cleanup()

	handler := NewChatHandler(zap.NewNop(), nil, wsURL, 1<<20)
	body, err := json.Marshal(userMessageRequest("hello"))
	require.NoError(t, err, "failed to marshal request")
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, "unexpected status code")
	require.Contains(t, rr.Body.String(), "\"stopReason\":\"max_tokens\"", "expected max_tokens stop reason in RUN_FINISHED")
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
	body, err := json.Marshal(userMessageRequest("hello"))
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	responseBody := rr.Body.String()
	require.Contains(t, responseBody, "\"type\":\"TEXT_MESSAGE_START\"",
		"SessionUpdate should trigger TEXT_MESSAGE_START SSE event")
	require.Contains(t, responseBody, "streamed-via-notification",
		"SessionUpdate delta should be flushed before Prompt() returns")
	require.Contains(t, responseBody, "\"type\":\"TEXT_MESSAGE_END\"",
		"TEXT_MESSAGE_END should close the open text message before RUN_FINISHED")
}

func TestChatHandlerContextEntriesAppendedAsPromptBlocks(t *testing.T) {
	agent := &mockACPAgent{}
	wsURL, cleanup := startMockACPWebSocketServer(t, agent)
	defer cleanup()

	handler := NewChatHandler(zap.NewNop(), nil, wsURL, 1<<20)

	reqBody, err := json.Marshal(ChatRequest{
		Messages: []aguitypes.Message{{Role: aguitypes.RoleUser, Content: "hi"}},
		Context: []aguitypes.Context{
			{Description: "selected_trace", Value: "trace-123"},
			{Value: "raw-context"},
		},
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(reqBody))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, "unexpected status code, body=%q", rr.Body.String())

	_, _, promptReq := agent.snapshot()
	require.NotNil(t, promptReq)
	require.Len(t, promptReq.Prompt, 3, "expected user text + two context blocks")
	require.Equal(t, "hi", promptReq.Prompt[0].Text.Text)
	require.Equal(t, "selected_trace:\ntrace-123", promptReq.Prompt[1].Text.Text)
	require.Equal(t, "raw-context", promptReq.Prompt[2].Text.Text)
}

func TestChatHandlerPromptErrorWriteFailure(t *testing.T) {
	agent := &mockACPAgent{promptErr: errors.New("prompt failed")}
	wsURL, cleanup := startMockACPWebSocketServer(t, agent)
	defer cleanup()

	handler := NewChatHandler(zap.NewNop(), nil, wsURL, 1<<20)
	body, err := json.Marshal(userMessageRequest("hello"))
	require.NoError(t, err, "failed to marshal request")
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(body))
	w := &failingResponseWriter{}

	handler.ServeHTTP(w, req)

	// Once startRun() writes to the SSE body (even if writes fail), the implicit
	// status becomes 200 OK; the handler should not surface a non-2xx afterward.
	require.Equal(t, http.StatusOK, w.status, "unexpected status code")
}
