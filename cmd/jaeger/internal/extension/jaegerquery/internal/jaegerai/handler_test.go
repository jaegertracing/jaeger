// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/acp-go-sdk"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

type mockACPAgent struct {
	mu sync.Mutex

	initializeReq *acp.InitializeRequest
	newSessionReq *acp.NewSessionRequest
	promptReq     *acp.PromptRequest
}

func (*mockACPAgent) Authenticate(context.Context, acp.AuthenticateRequest) (acp.AuthenticateResponse, error) {
	return acp.AuthenticateResponse{}, nil
}

func (a *mockACPAgent) Initialize(_ context.Context, params acp.InitializeRequest) (acp.InitializeResponse, error) {
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
	a.mu.Lock()
	defer a.mu.Unlock()
	cp := params
	a.newSessionReq = &cp
	return acp.NewSessionResponse{SessionId: "sess-test"}, nil
}

func (a *mockACPAgent) Prompt(ctx context.Context, params acp.PromptRequest) (acp.PromptResponse, error) {
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

	handler := NewChatHandler(zap.NewNop(), nil, wsURL)

	reqBody, err := json.Marshal(ChatRequest{Prompt: "trace for service checkout"})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(reqBody))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status code: got %d want %d body=%q", rr.Code, http.StatusOK, rr.Body.String())
	}

	initReq, sessionReq, promptReq := agent.snapshot()
	if initReq == nil || sessionReq == nil || promptReq == nil {
		t.Fatalf("expected initialize/new session/prompt requests to be captured, got init=%v session=%v prompt=%v", initReq != nil, sessionReq != nil, promptReq != nil)
	}

	if initReq.ProtocolVersion != acp.ProtocolVersionNumber {
		t.Fatalf("initialize protocol version mismatch: got %d want %d", initReq.ProtocolVersion, acp.ProtocolVersionNumber)
	}
	if initReq.ClientCapabilities.Fs.ReadTextFile || initReq.ClientCapabilities.Fs.WriteTextFile || initReq.ClientCapabilities.Terminal {
		t.Fatalf("unexpected client capabilities in initialize: %+v", initReq.ClientCapabilities)
	}
	if initReq.ClientInfo == nil || initReq.ClientInfo.Name != "jaeger-ai-gateway" || initReq.ClientInfo.Version != "0.1.0" {
		t.Fatalf("unexpected client info in initialize: %+v", initReq.ClientInfo)
	}

	if sessionReq.Cwd != "/" {
		t.Fatalf("session/new cwd mismatch: got %q want %q", sessionReq.Cwd, "/")
	}
	if len(sessionReq.McpServers) != 0 {
		t.Fatalf("expected no MCP servers in session/new, got %d", len(sessionReq.McpServers))
	}

	if promptReq.SessionId != "sess-test" {
		t.Fatalf("prompt sessionId mismatch: got %q want %q", promptReq.SessionId, "sess-test")
	}
	if len(promptReq.Prompt) != 1 || promptReq.Prompt[0].Text == nil {
		t.Fatalf("prompt content mismatch: %+v", promptReq.Prompt)
	}
	if got, want := promptReq.Prompt[0].Text.Text, "trace for service checkout"; got != want {
		t.Fatalf("prompt text mismatch: got %q want %q", got, want)
	}

	if ct := rr.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Fatalf("content type mismatch: got %q", ct)
	}
	if cc := rr.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Fatalf("cache-control mismatch: got %q", cc)
	}
	if conn := rr.Header().Get("Connection"); conn != "keep-alive" {
		t.Fatalf("connection header mismatch: got %q", conn)
	}
}
