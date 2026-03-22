// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/coder/acp-go-sdk"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
)

const (
	endOfTurnMarker        = "__END_OF_TURN__"
	maxChatRequestBodySize = 1 << 20 // 1 MiB
)

// ChatRequest is the incoming payload
type ChatRequest struct {
	Prompt string `json:"prompt"`
}

// ChatHandler manages the AI gateway requests
type ChatHandler struct {
	Logger       *zap.Logger
	QueryService *querysvc.QueryService
	sidecarWSURL string
}

func NewChatHandler(logger *zap.Logger, queryService *querysvc.QueryService, sidecarWSURL string) *ChatHandler {
	return &ChatHandler{Logger: logger, QueryService: queryService, sidecarWSURL: sidecarWSURL}
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

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
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
	}
	// Build an ACP client-side connection over the websocket adapter.
	acpConn := acp.NewClientSideConnection(clientImpl, adapter, adapter)

	acpCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	_, err = acpConn.Initialize(acpCtx, acp.InitializeRequest{
		ProtocolVersion: acp.ProtocolVersionNumber,
		ClientCapabilities: acp.ClientCapabilities{
			Fs:       acp.FileSystemCapability{ReadTextFile: false, WriteTextFile: false},
			Terminal: false,
		},
		ClientInfo: &acp.Implementation{
			Name:    "jaeger-ai-gateway",
			Version: "0.1.0",
		},
	})
	if err != nil {
		if _, writeErr := fmt.Fprintf(w, "Error initializing agent: %v\n", err); writeErr != nil {
			h.Logger.Warn("Failed to write initialize error response", zap.Error(writeErr))
		}
		return
	}

	sess, err := acpConn.NewSession(acpCtx, acp.NewSessionRequest{
		Cwd:        "/",
		McpServers: []acp.McpServer{},
	})
	if err != nil {
		if _, writeErr := fmt.Fprintf(w, "Error creating session: %v\n", err); writeErr != nil {
			h.Logger.Warn("Failed to write new session error response", zap.Error(writeErr))
		}
		return
	}

	// This blocks until the sidecar completes the ACP prompt turn.
	_, err = acpConn.Prompt(acpCtx, acp.PromptRequest{
		SessionId: sess.SessionId,
		Prompt:    []acp.ContentBlock{acp.TextBlock(req.Prompt)},
	})
	if err != nil {
		if _, writeErr := fmt.Fprintf(w, "Error starting prompt: %v\n", err); writeErr != nil {
			h.Logger.Warn("Failed to write prompt error response", zap.Error(writeErr))
		}
		return
	}

	// Wait for explicit end-of-turn marker from the sidecar, with timeout fallback.
	clientImpl.waitForTurnCompletion(acpCtx, 2*time.Second)
}
