// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package aireconciler

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

// stubAgent is the minimal acp.Agent implementation needed by the probe —
// only Initialize and Cancel are exercised. Every other method returns a
// zero value and nil error so the SDK does not blow up if it routes
// something here that we don't expect.
type stubAgent struct {
	mu         sync.Mutex
	initCount  int
	initErr    error
	asc        *acp.AgentSideConnection
	initBlocks bool // when true, Initialize blocks until ctx is done (to exercise timeout)
}

func (*stubAgent) Authenticate(context.Context, acp.AuthenticateRequest) (acp.AuthenticateResponse, error) {
	return acp.AuthenticateResponse{}, nil
}

func (a *stubAgent) Initialize(ctx context.Context, params acp.InitializeRequest) (acp.InitializeResponse, error) {
	a.mu.Lock()
	a.initCount++
	blocks := a.initBlocks
	initErr := a.initErr
	a.mu.Unlock()

	if blocks {
		<-ctx.Done()
		return acp.InitializeResponse{}, ctx.Err()
	}
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
	return acp.NewSessionResponse{SessionId: "sess-probe"}, nil
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
		agent.mu.Lock()
		agent.asc = asc
		agent.mu.Unlock()
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

func TestACPProbe_SucceedsAgainstReachableSidecar(t *testing.T) {
	agent := &stubAgent{}
	wsURL := startMockACPServer(t, agent)

	r, err := New(Config{
		AgentURL: wsURL,
		Interval: 20 * time.Millisecond,
		Timeout:  500 * time.Millisecond,
		Logger:   zap.NewNop(),
	})
	require.NoError(t, err)
	r.Start(t.Context())
	defer r.Stop()

	require.Eventually(t, r.Current, time.Second, 10*time.Millisecond,
		"reconciler should flip to true once the sidecar responds to initialize")

	agent.mu.Lock()
	require.Positive(t, agent.initCount, "sidecar should have received at least one initialize")
	agent.mu.Unlock()
}

func TestACPProbe_FailsAgainstNonexistentSidecar(t *testing.T) {
	r, err := New(Config{
		AgentURL: "ws://127.0.0.1:1", // closed port
		Interval: 20 * time.Millisecond,
		Timeout:  200 * time.Millisecond,
		Logger:   zap.NewNop(),
	})
	require.NoError(t, err)
	r.Start(t.Context())
	defer r.Stop()

	// Probe failures don't flip the state — initial state is already false —
	// so the assertion is that after running for a few intervals the
	// reconciler is still false (and didn't crash).
	time.Sleep(150 * time.Millisecond)
	require.False(t, r.Current())
}

func TestACPProbe_RecordsInitializeError(t *testing.T) {
	agent := &stubAgent{initErr: errors.New("agent rejected initialize")}
	wsURL := startMockACPServer(t, agent)

	r, err := New(Config{
		AgentURL: wsURL,
		Interval: 20 * time.Millisecond,
		Timeout:  500 * time.Millisecond,
		Logger:   zap.NewNop(),
	})
	require.NoError(t, err)
	r.Start(t.Context())
	defer r.Stop()

	// Initialize rejection means probe returns error, capability stays false.
	time.Sleep(150 * time.Millisecond)
	require.False(t, r.Current())

	agent.mu.Lock()
	require.Positive(t, agent.initCount, "sidecar should have received initialize attempts")
	agent.mu.Unlock()
}
