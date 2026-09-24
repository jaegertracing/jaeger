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
	"go.opentelemetry.io/collector/config/configopaque"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/jaegerai"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/jaegerai/internal/acptest"
)

// stubAgent is the minimal acp.Agent implementation needed by the check: only
// Initialize is exercised, and the embedded UnimplementedAgent answers
// anything else the SDK happens to route.
type stubAgent struct {
	acptest.UnimplementedAgent

	mu             sync.Mutex
	initCount      int
	initErr        error
	lastAuthHeader string
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

func startMockACPServer(t *testing.T, agent *stubAgent) string {
	t.Helper()
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agent.mu.Lock()
		agent.lastAuthHeader = r.Header.Get("X-Secret-Key")
		agent.mu.Unlock()

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

	err := NewACPCheck(wsURL, nil, zap.NewNop())(t.Context())
	require.NoError(t, err)

	agent.mu.Lock()
	require.Equal(t, 1, agent.initCount, "sidecar should have received exactly one initialize")
	agent.mu.Unlock()
}

func TestACPCheck_FailsWhenDialFails(t *testing.T) {
	err := NewACPCheck("ws://127.0.0.1:1", nil, zap.NewNop())(t.Context()) // closed port
	require.ErrorContains(t, err, "dial:")
}

func TestACPCheck_FailsWhenInitializeRejected(t *testing.T) {
	agent := &stubAgent{initErr: errors.New("agent rejected initialize")}
	wsURL := startMockACPServer(t, agent)

	err := NewACPCheck(wsURL, nil, zap.NewNop())(t.Context())
	require.ErrorContains(t, err, "initialize:")
}

// The health checker dials the agent independently of the chat endpoint. If it
// were to skip the configured headers, an authenticated agent would reject it
// and the gateway would never advertise the assistant, even though chat itself
// works — a failure that looks like the agent being down.
func TestACPCheck_SendsConfiguredHeaders(t *testing.T) {
	agent := &stubAgent{}
	wsURL := startMockACPServer(t, agent)

	var headers configopaque.MapList
	headers.Set("X-Secret-Key", "s3cret")

	require.NoError(t, NewACPCheck(wsURL, headers, zap.NewNop())(t.Context()))

	agent.mu.Lock()
	defer agent.mu.Unlock()
	require.Equal(t, "s3cret", agent.lastAuthHeader, "health check must send the configured header")
}
