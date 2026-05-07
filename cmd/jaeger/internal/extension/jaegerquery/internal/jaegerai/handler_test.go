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
	"github.com/stretchr/testify/assert"
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

// newAGUIRequest builds a minimal AG-UI RunAgentInput carrying a single
// user message with the supplied prompt text. Tests that need richer
// payloads (tools, context, thread/run ids) construct ChatRequest
// inline.
func newAGUIRequest(prompt string) ChatRequest {
	return ChatRequest{
		Messages: []aguitypes.Message{{Role: aguitypes.RoleUser, Content: prompt}},
	}
}

func TestChatHandlerSendsACPProtocolRequests(t *testing.T) {
	agent := &mockACPAgent{}
	wsURL, cleanup := startMockACPWebSocketServer(t, agent)
	defer cleanup()

	handler := NewChatHandler(zap.NewNop(), nil, wsURL, "/jaeger", 1<<20)

	reqBody, err := json.Marshal(newAGUIRequest("trace for service checkout"))
	require.NoError(t, err, "failed to marshal request")

	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(reqBody))
	req.Host = "gateway.example:16686"
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
	require.Empty(t, sessionReq.McpServers,
		"PR2 must not advertise gateway-hosted MCP servers; contextual tools ride ACP extension methods now")
	require.Empty(t, sessionReq.Meta,
		"Meta must be omitted when the AG-UI request carries no tools")

	require.EqualValues(t, "sess-test", promptReq.SessionId, "prompt sessionId mismatch")
	require.Len(t, promptReq.Prompt, 1, "prompt content length mismatch")
	require.NotNil(t, promptReq.Prompt[0].Text, "prompt text should not be nil")
	require.Equal(t, "trace for service checkout", promptReq.Prompt[0].Text.Text, "prompt text mismatch")

	require.Equal(t, "text/event-stream", rr.Header().Get("Content-Type"), "content type mismatch")
	require.Equal(t, "no-cache", rr.Header().Get("Cache-Control"), "cache-control mismatch")
	require.Empty(t, rr.Header().Get("Connection"), "Connection is a hop-by-hop header managed by net/http")
}

func TestChatHandlerAppendsContextEntriesToPromptBlocks(t *testing.T) {
	agent := &mockACPAgent{}
	wsURL, cleanup := startMockACPWebSocketServer(t, agent)
	defer cleanup()

	handler := NewChatHandler(zap.NewNop(), nil, wsURL, "", 1<<20)

	reqBody, err := json.Marshal(ChatRequest{
		Messages: []aguitypes.Message{{Role: aguitypes.RoleUser, Content: "where is the latency?"}},
		Context: []aguitypes.Context{
			{Description: "trace_id", Value: "abc-123"},
			{Value: "user pressed checkout"},
		},
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(reqBody))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	_, _, promptReq := agent.snapshot()
	require.NotNil(t, promptReq)
	require.Len(t, promptReq.Prompt, 3, "expected one user-message block plus two context blocks")
	require.Equal(t, "where is the latency?", promptReq.Prompt[0].Text.Text)
	require.Equal(t, "trace_id:\nabc-123", promptReq.Prompt[1].Text.Text)
	require.Equal(t, "user pressed checkout", promptReq.Prompt[2].Text.Text)
}

func TestChatHandlerAttachesContextualToolsToMetaAndStore(t *testing.T) {
	// AG-UI tools on the request should land both on NewSessionRequest._meta
	// (so the sidecar can register them with the LLM) and in
	// ContextualToolsStore (so handleJaegerToolCall can resolve callbacks).
	// Tool names must be UIToolPrefix-prefixed in both places, and the
	// store entry must be cleared once the turn ends.
	agent := &mockACPAgent{}
	wsURL, cleanup := startMockACPWebSocketServer(t, agent)
	defer cleanup()

	store := NewContextualToolsStore()
	handler := NewChatHandler(zap.NewNop(), store, wsURL, "", 1<<20)

	reqBody, err := json.Marshal(ChatRequest{
		Messages: []aguitypes.Message{{Role: aguitypes.RoleUser, Content: "hello"}},
		Tools: []aguitypes.Tool{
			{Name: "render_chart", Description: "draw a chart", Parameters: map[string]any{"type": "object"}},
		},
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(reqBody))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	_, sessionReq, _ := agent.snapshot()
	require.NotNil(t, sessionReq)
	require.NotEmpty(t, sessionReq.Meta, "Meta must be populated when the request carries tools")
	// Meta is map[string]any, so after JSON round-trip the contextual tools
	// payload arrives as a generic decoded shape rather than typed slices.
	payload, ok := sessionReq.Meta[ContextualToolsMetaKey].(map[string]any)
	require.True(t, ok, "Meta must contain the contextual tools key as a map")
	tools, ok := payload["tools"].([]any)
	require.True(t, ok, "Meta tools entry must decode as a JSON array")
	require.Len(t, tools, 1)
	first, ok := tools[0].(map[string]any)
	require.True(t, ok, "tool entry must decode as an object")
	assert.Equal(t, UIToolPrefix+"render_chart", first["name"],
		"the gateway must prepend UIToolPrefix before exposing the tool")

	// SetForSession was called with the same prefixed payload, and
	// DeleteForSession must have run via defer once the turn finished.
	assert.Nil(t, store.GetContextualToolsForSession("sess-test"),
		"store entry should be cleared after the turn ends")
}

func TestChatHandlerOmitsMetaWhenNoTools(t *testing.T) {
	agent := &mockACPAgent{}
	wsURL, cleanup := startMockACPWebSocketServer(t, agent)
	defer cleanup()

	handler := NewChatHandler(zap.NewNop(), NewContextualToolsStore(), wsURL, "", 1<<20)

	reqBody, err := json.Marshal(newAGUIRequest("hello"))
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(reqBody))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	_, sessionReq, _ := agent.snapshot()
	require.NotNil(t, sessionReq)
	require.Empty(t, sessionReq.Meta,
		"Meta must stay nil/empty when no tools are sent so the sidecar does not see a stale snapshot")
}

func TestChatHandlerEmitsRunFinishedWithStopReason(t *testing.T) {
	agent := &mockACPAgent{promptStopReason: acp.StopReasonMaxTokens}
	wsURL, cleanup := startMockACPWebSocketServer(t, agent)
	defer cleanup()

	handler := NewChatHandler(zap.NewNop(), nil, wsURL, "", 1<<20)
	reqBody, err := json.Marshal(newAGUIRequest("hello"))
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(reqBody))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	events := parseSSEEvents(t, rr.Body.String())
	types := eventTypes(events)
	require.Equal(t, []string{"RUN_STARTED", "RUN_FINISHED"}, types,
		"a turn with no streamed text should emit just the lifecycle frames")
	assert.Equal(t, "max_tokens", events[1]["stopReason"])
}

func TestChatHandlerPropagatesThreadAndRunIDs(t *testing.T) {
	agent := &mockACPAgent{}
	wsURL, cleanup := startMockACPWebSocketServer(t, agent)
	defer cleanup()

	handler := NewChatHandler(zap.NewNop(), nil, wsURL, "", 1<<20)

	reqBody, err := json.Marshal(ChatRequest{
		ThreadID: "thread-xyz",
		RunID:    "run-789",
		Messages: []aguitypes.Message{{Role: aguitypes.RoleUser, Content: "hello"}},
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(reqBody))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	events := parseSSEEvents(t, rr.Body.String())
	require.NotEmpty(t, events)
	assert.Equal(t, "thread-xyz", events[0]["threadId"])
	assert.Equal(t, "run-789", events[0]["runId"])
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
	h := NewChatHandler(zap.NewNop(), store, "ws://localhost:1", "/jaeger", 512)
	require.Equal(t, int64(512), h.maxRequestBodySize, "expected configured maxRequestBodySize")
	require.Equal(t, "ws://localhost:1", h.sidecarWSURL, "expected configured sidecarWSURL")
	require.Equal(t, "/jaeger", h.basePath, "expected configured basePath")
	require.Same(t, store, h.ctxTools, "expected configured ctxTools store")
}

func TestChatHandlerMethodNotAllowed(t *testing.T) {
	handler := NewChatHandler(zap.NewNop(), nil, "ws://127.0.0.1:1", "", 1<<20)
	req := httptest.NewRequest(http.MethodGet, "/api/ai/chat", http.NoBody)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusMethodNotAllowed, rr.Code, "unexpected status code")
}

func TestChatHandlerBadRequest(t *testing.T) {
	handler := NewChatHandler(zap.NewNop(), nil, "ws://127.0.0.1:1", "", 1<<20)
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", strings.NewReader("{"))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code, "unexpected status code")
}

func TestChatHandlerEmptyPrompt(t *testing.T) {
	handler := NewChatHandler(zap.NewNop(), nil, "ws://127.0.0.1:1", "", 1<<20)
	body, err := json.Marshal(newAGUIRequest("   "))
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code, "unexpected status code")
	require.Contains(t, rr.Body.String(), "prompt is required")
}

func TestChatHandlerNoUserMessage(t *testing.T) {
	handler := NewChatHandler(zap.NewNop(), nil, "ws://127.0.0.1:1", "", 1<<20)
	body, err := json.Marshal(ChatRequest{
		Messages: []aguitypes.Message{{Role: "assistant", Content: "no user message in this run"}},
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code,
		"a RunAgentInput with no user message must fail with 400, not silently succeed")
	require.Contains(t, rr.Body.String(), "prompt is required")
}

func TestChatHandlerRequestBodyTooLarge(t *testing.T) {
	handler := NewChatHandler(zap.NewNop(), nil, "ws://127.0.0.1:1", "", 10)
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", strings.NewReader(`{"messages":[{"role":"user","content":"this body exceeds the 10 byte limit"}]}`))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusRequestEntityTooLarge, rr.Code, "unexpected status code")
}

func TestChatHandlerDialFailure(t *testing.T) {
	handler := NewChatHandler(zap.NewNop(), nil, "ws://127.0.0.1:1", "", 1<<20)
	body, err := json.Marshal(newAGUIRequest("hello"))
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

	handler := NewChatHandler(zap.NewNop(), nil, wsURL, "", 1<<20)
	body, err := json.Marshal(newAGUIRequest("hello"))
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

	handler := NewChatHandler(zap.NewNop(), nil, wsURL, "", 1<<20)
	body, err := json.Marshal(newAGUIRequest("hello"))
	require.NoError(t, err, "failed to marshal request")
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadGateway, rr.Code, "unexpected status code, body=%q", rr.Body.String())
	require.Contains(t, rr.Body.String(), "Error creating session", "expected session error message")
}

func TestChatHandlerPromptError(t *testing.T) {
	// Once the SSE response has been committed (Content-Type set, RUN_STARTED
	// emitted), a Prompt failure cannot rewrite the status code — instead the
	// handler must emit a RUN_ERROR event so the AG-UI client can finalise.
	agent := &mockACPAgent{promptErr: errors.New("prompt failed")}
	wsURL, cleanup := startMockACPWebSocketServer(t, agent)
	defer cleanup()

	handler := NewChatHandler(zap.NewNop(), nil, wsURL, "", 1<<20)
	body, err := json.Marshal(newAGUIRequest("hello"))
	require.NoError(t, err, "failed to marshal request")
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code,
		"SSE response is already committed by the time Prompt is called, so the handler must signal failure inline")
	events := parseSSEEvents(t, rr.Body.String())
	types := eventTypes(events)
	require.Contains(t, types, "RUN_ERROR")
	for _, e := range events {
		if e["type"] == "RUN_ERROR" {
			require.Contains(t, e["message"], "Error starting prompt")
			return
		}
	}
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

	handler := NewChatHandler(zap.NewNop(), nil, wsURL, "", 1<<20)
	body, err := json.Marshal(newAGUIRequest("hello"))
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "streamed-via-notification",
		"streamed delta must be flushed (inside a TEXT_MESSAGE_CONTENT frame) before Prompt() returns")
}

func TestChatHandlerPromptErrorWriteFailure(t *testing.T) {
	// When the response writer fails after headers have been committed, the
	// handler still completes via failRun → write attempts → silent close.
	// The recorder reports whatever status the underlying writer captured;
	// here, Write sets it to 200 on the first call.
	agent := &mockACPAgent{promptErr: errors.New("prompt failed")}
	wsURL, cleanup := startMockACPWebSocketServer(t, agent)
	defer cleanup()

	handler := NewChatHandler(zap.NewNop(), nil, wsURL, "", 1<<20)
	body, err := json.Marshal(newAGUIRequest("hello"))
	require.NoError(t, err, "failed to marshal request")
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(body))
	w := &failingFlusherResponseWriter{}

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.status,
		"Write captures status 200 when the first SSE frame attempt fails; no later WriteHeader override is allowed")
}
