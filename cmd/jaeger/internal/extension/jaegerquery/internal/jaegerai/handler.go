// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	acp "github.com/coder/acp-go-sdk"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/version"
)

const (
	// endOfTurnMarker works around a race in acp-go-sdk where SessionUpdate
	// callbacks may still be running when Prompt() returns, because notifications
	// are dispatched via goroutines (go c.handleInbound in connection.go).
	// Without this marker, the HTTP response could close before the final text
	// chunk is flushed. The sidecar sends this as the last SessionUpdate.
	endOfTurnMarker           = "__END_OF_TURN__"
	maxChatRequestBodySize    = 1 << 20 // 1 MiB
	defaultWaitForTurnTimeout = 180 * time.Second
)

// ChatRequest is the incoming payload
type ChatRequest struct {
	Prompt string `json:"prompt"`
}

// ChatHandler manages the AI gateway requests
type ChatHandler struct {
	Logger             *zap.Logger
	sidecarWSURL       string
	waitForTurnTimeout time.Duration
}

func NewChatHandler(logger *zap.Logger, sidecarWSURL string, waitForTurnTimeout time.Duration) *ChatHandler {
	if waitForTurnTimeout <= 0 {
		waitForTurnTimeout = defaultWaitForTurnTimeout
	}

	return &ChatHandler{
		Logger:             logger,
		sidecarWSURL:       sidecarWSURL,
		waitForTurnTimeout: waitForTurnTimeout,
	}
}

func (h *ChatHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST method is supported", http.StatusMethodNotAllowed)
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
	acpCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	adapter, err := DialWsAdapter(acpCtx, h.sidecarWSURL)
	if err != nil {
		h.Logger.Error("Failed to dial ACP sidecar", zap.Error(err))
		http.Error(w, "Failed to connect to agent backend", http.StatusBadGateway)
		return
	}
	defer adapter.Close()

	clientImpl := &streamingClient{
		requestCtx: ctx,
		w:          w,
		flusher:    flusher,
		doneCh:     make(chan struct{}),
	}
	// Build an ACP client-side connection over the websocket adapter.
	acpConn := acp.NewClientSideConnection(clientImpl, adapter, adapter)

	clientVersion := version.Get().GitVersion
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
		w.WriteHeader(http.StatusBadGateway)
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
		w.WriteHeader(http.StatusBadGateway)
		if _, writeErr := fmt.Fprintf(w, "Error creating session: %v\n", err); writeErr != nil {
			h.Logger.Warn("Failed to write new session error response", zap.Error(writeErr))
		}
		return
	}

	// Prompt blocks until the sidecar completes the ACP turn. During processing,
	// SessionUpdate callbacks may stream text to the HTTP response via clientImpl.
	_, err = acpConn.Prompt(acpCtx, acp.PromptRequest{
		SessionId: sess.SessionId,
		Prompt:    []acp.ContentBlock{acp.TextBlock(req.Prompt)},
	})
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		if _, writeErr := fmt.Fprintf(w, "Error starting prompt: %v\n", err); writeErr != nil {
			h.Logger.Warn("Failed to write prompt error response", zap.Error(writeErr))
		}
		return
	}

	// Wait for explicit end-of-turn marker from the sidecar, with configured timeout fallback.
	clientImpl.waitForTurnCompletion(acpCtx, h.waitForTurnTimeout)
}
