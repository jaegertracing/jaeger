// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package aihealth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/jaegerai"
)

// stubAgent is the minimal acp.Agent implementation needed by the check —
// only Initialize is exercised. Every other method returns a zero value and
// nil error so the SDK does not blow up if it routes something unexpected.
type stubAgent struct {
	mu        sync.Mutex
	initCount int
	initErr   error
}

func (*stubAgent) Authenticate(context.Context, acp.AuthenticateRequest) (acp.AuthenticateResponse, error) {
	return acp.AuthenticateResponse{}, nil
}

func (a *stubAgent) Initialize(_ context.Context, params acp.InitializeRequest) (acp.InitializeResponse, error) {
	a.mu.Lock()
	a.initCount++
	initErr := a.initErr
	a.mu.Unlock()
	if initErr != nil {
		return acp.InitializeResponse{}, initErr
	}
	return acp.InitializeResponse{ProtocolVersion: params.ProtocolVersion}, nil
}

func (*stubAgent) Cancel(context.Context, acp.CancelNotification) error { return nil }

func (*stubAgent) CloseSession(context.Context, acp.CloseSessionRequest) (acp.CloseSessionResponse, error) {
	return acp.CloseSessionResponse{}, nil
}

func (*stubAgent) ListSessions(context.Context, acp.ListSessionsRequest) (acp.ListSessionsResponse, error) {
	return acp.ListSessionsResponse{}, nil
}

func (*stubAgent) NewSession(context.Context, acp.NewSessionRequest) (acp.NewSessionResponse, error) {
	return acp.NewSessionResponse{SessionId: "sess-check"}, nil
}

func (*stubAgent) Prompt(context.Context, acp.PromptRequest) (acp.PromptResponse, error) {
	return acp.PromptResponse{StopReason: acp.StopReasonEndTurn}, nil
}

func (*stubAgent) ResumeSession(context.Context, acp.ResumeSessionRequest) (acp.ResumeSessionResponse, error) {
	return acp.ResumeSessionResponse{}, nil
}

func (*stubAgent) SetSessionConfigOption(context.Context, acp.SetSessionConfigOptionRequest) (acp.SetSessionConfigOptionResponse, error) {
	return acp.SetSessionConfigOptionResponse{}, nil
}

func (*stubAgent) SetSessionMode(context.Context, acp.SetSessionModeRequest) (acp.SetSessionModeResponse, error) {
	return acp.SetSessionModeResponse{}, nil
}

func startMockACPServer(t *testing.T, agent *stubAgent) string {
	t.Helper()
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		adapter := jaegerai.NewWsAdapter(conn, zap.NewNop())
		asc := acp.NewAgentSideConnection(agent, adapter, adapter)
		<-asc.Done()
	}))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Config.Shutdown(ctx)
		srv.Close()
	})
	return "ws" + strings.TrimPrefix(srv.URL, "http")
}

func TestACPCheck_SucceedsAgainstReachableSidecar(t *testing.T) {
	agent := &stubAgent{}
	wsURL := startMockACPServer(t, agent)

	err := NewACPCheck(wsURL, zap.NewNop())(t.Context())
	require.NoError(t, err)

	agent.mu.Lock()
	require.Equal(t, 1, agent.initCount, "sidecar should have received exactly one initialize")
	agent.mu.Unlock()
}

func TestACPCheck_FailsWhenDialFails(t *testing.T) {
	err := NewACPCheck("ws://127.0.0.1:1", zap.NewNop())(t.Context()) // closed port
	require.ErrorContains(t, err, "dial:")
}

func TestACPCheck_FailsWhenInitializeRejected(t *testing.T) {
	agent := &stubAgent{initErr: errors.New("agent rejected initialize")}
	wsURL := startMockACPServer(t, agent)

	err := NewACPCheck(wsURL, zap.NewNop())(t.Context())
	require.ErrorContains(t, err, "initialize:")
}
