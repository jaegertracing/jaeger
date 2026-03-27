// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	aguitypes "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
	acp "github.com/coder/acp-go-sdk"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/version"
)

const (
	endOfTurnMarker        = "__END_OF_TURN__"
	maxChatRequestBodySize = 1 << 20 // 1 MiB
)

// ChatRequest is the incoming AG-UI payload.
type ChatRequest = aguitypes.RunAgentInput

// ChatHandler manages the AI gateway requests
type ChatHandler struct {
	Logger       *zap.Logger
	QueryService *querysvc.QueryService
	sidecarWSURL string
	toolsMu      sync.RWMutex
	toolsBySess  map[string][]json.RawMessage
}

func NewChatHandler(logger *zap.Logger, queryService *querysvc.QueryService, sidecarWSURL string) *ChatHandler {
	return &ChatHandler{
		Logger:       logger,
		QueryService: queryService,
		sidecarWSURL: sidecarWSURL,
		toolsBySess:  make(map[string][]json.RawMessage),
	}
}

func (h *ChatHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Limit the size of the request body to prevent memory/CPU abuse.
	r.Body = http.MaxBytesReader(w, r.Body, maxChatRequestBodySize)
	defer r.Body.Close()

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	prompt, err := latestUserMessageText(req.Messages)
	if err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	promptBlocks := []acp.ContentBlock{acp.TextBlock(prompt)}
	for _, ctxText := range contextTextEntries(req.Context) {
		promptBlocks = append(promptBlocks, acp.TextBlock(ctxText))
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ctx := r.Context()
	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	conn, resp, err := dialer.DialContext(ctx, h.sidecarWSURL, nil)
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		h.Logger.Error("Failed to dial ACP sidecar", zap.Error(err))
		http.Error(w, "Failed to connect to agent backend", http.StatusBadGateway)
		return
	}
	defer conn.Close()

	adapter := NewWsAdapter(conn)

	clientImpl := &streamingClient{
		requestCtx: ctx,
		w:          w,
		flusher:    flusher,
		doneCh:     make(chan struct{}),
		runID:      req.RunID,
	}
	clientImpl.startRun()

	// Build an ACP client-side connection over the websocket adapter.
	acpConn := acp.NewClientSideConnection(clientImpl, adapter, adapter)

	acpCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	clientVersion := version.Get().GitVersion
	if clientVersion == "" {
		clientVersion = "dev"
	}
	_, err = acpConn.Initialize(acpCtx, acp.InitializeRequest{
		ProtocolVersion: acp.ProtocolVersionNumber,
		ClientCapabilities: acp.ClientCapabilities{
			Fs:       acp.FileSystemCapability{ReadTextFile: false, WriteTextFile: false},
			Terminal: false,
		},
		ClientInfo: &acp.Implementation{
			Name:    "jaeger-ai-gateway",
			Version: clientVersion,
		},
	})
	if err != nil {
		clientImpl.failRun(fmt.Sprintf("Error initializing agent: %v", err))
		clientImpl.signalDone()
		return
	}

	sess, err := acpConn.NewSession(acpCtx, acp.NewSessionRequest{
		Cwd:        "/",
		McpServers: []acp.McpServer{},
	})
	if err != nil {
		clientImpl.failRun(fmt.Sprintf("Error creating session: %v", err))
		clientImpl.signalDone()
		return
	}

	// Track tools by ACP session and publish latest snapshot through QueryService for MCP reads.
	toolsRaw := encodeToolsAsRaw(req.Tools)
	h.updateSessionTools(string(sess.SessionId), toolsRaw)
	if h.QueryService != nil {
		h.QueryService.SetContextualToolsForSession(string(sess.SessionId), toolsRaw)
	}

	// This blocks until the sidecar completes the ACP prompt turn.
	_, err = acpConn.Prompt(acpCtx, acp.PromptRequest{
		SessionId: sess.SessionId,
		Prompt:    promptBlocks,
	})
	if err != nil {
		clientImpl.failRun(fmt.Sprintf("Error starting prompt: %v", err))
		clientImpl.signalDone()
		return
	}

	// Wait for explicit end-of-turn marker from the sidecar, with a 3 mins timeout fallback.
	clientImpl.waitForTurnCompletion(acpCtx, 180*time.Second)
}

func (h *ChatHandler) updateSessionTools(sessionID string, tools []json.RawMessage) {
	h.toolsMu.Lock()
	defer h.toolsMu.Unlock()

	cloned := make([]json.RawMessage, len(tools))
	for i, raw := range tools {
		cloned[i] = append(json.RawMessage(nil), raw...)
	}
	h.toolsBySess[sessionID] = cloned
}
