// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	acp "github.com/coder/acp-go-sdk"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/version"
)

// ChatRequest is the incoming payload
type ChatRequest struct {
	Prompt string `json:"prompt"`
}

// ChatHandler manages the AI gateway requests
type ChatHandler struct {
	Logger             *zap.Logger
	ctxTools           *ContextualToolsStore
	sidecarWSURL       string
	basePath           string
	maxRequestBodySize int64
}

// NewChatHandler wires the chat endpoint against a sidecar WebSocket URL.
// ctxTools may be nil in tests that do not exercise contextual tooling.
// basePath is the jaeger-query base path (empty or "/" mean no prefix);
// it is normalized so concatenations cannot produce a double slash.
func NewChatHandler(logger *zap.Logger, ctxTools *ContextualToolsStore, sidecarWSURL, basePath string, maxRequestBodySize int64) *ChatHandler {
	return &ChatHandler{
		Logger:             logger,
		ctxTools:           ctxTools,
		sidecarWSURL:       sidecarWSURL,
		basePath:           normalizeBasePath(basePath),
		maxRequestBodySize: maxRequestBodySize,
	}
}

func (h *ChatHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST method is supported", http.StatusMethodNotAllowed)
		return
	}

	// Limit the size of the request body to prevent memory/CPU abuse.
	r.Body = http.MaxBytesReader(w, r.Body, h.maxRequestBodySize)
	defer r.Body.Close()

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			http.Error(w, "Request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(req.Prompt) == "" {
		http.Error(w, "prompt is required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	acpCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	adapter, err := DialWsAdapter(acpCtx, h.sidecarWSURL, h.Logger)
	if err != nil {
		http.Error(w, "Failed to connect to agent backend", http.StatusBadGateway)
		return
	}
	defer adapter.Close()

	clientImpl := newStreamingClient(ctx, w)
	// Build the ACP connection ourselves so the inbound dispatcher can
	// route both standard ACP methods (session/update etc.) and our
	// extension method (ExtMethodJaegerToolCall) — the SDK's
	// NewClientSideConnection has a hardcoded dispatcher that returns
	// MethodNotFound for any extension method we add.
	acpConn := acp.NewConnection(newDispatcher(clientImpl, h.Logger), adapter, adapter)

	if _, err = acp.SendRequest[acp.InitializeResponse](acpConn, acpCtx, acp.AgentMethodInitialize, acp.InitializeRequest{
		ProtocolVersion: acp.ProtocolVersionNumber,
		ClientCapabilities: acp.ClientCapabilities{
			Fs:       acp.FileSystemCapability{ReadTextFile: false, WriteTextFile: false},
			Terminal: false,
		},
		ClientInfo: &acp.Implementation{
			Name:    "jaeger-ai-gateway",
			Version: version.Get().GitVersion,
		},
	}); err != nil {
		http.Error(w, fmt.Sprintf("Error initializing agent: %v", err), http.StatusBadGateway)
		return
	}

	sess, err := acp.SendRequest[acp.NewSessionResponse](acpConn, acpCtx, acp.AgentMethodSessionNew, acp.NewSessionRequest{
		Cwd: "/",
		// McpServers must be non-nil for ACP validation. PR1 advertises no
		// MCP servers (contextual tools ride the ACP extension method
		// instead); PR2 may add the built-in jaeger MCP server here.
		McpServers: []acp.McpServer{},
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("Error creating session: %v", err), http.StatusBadGateway)
		return
	}

	// Set streaming headers just before Prompt(), since SessionUpdate callbacks
	// may start writing to the response during this call.
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")

	// TODO(PR2): wire AG-UI tools into NewSessionRequest.Meta and into
	// h.ctxTools so the sidecar can register contextual tools with Gemini
	// and dispatch them back through ExtMethodJaegerToolCall.

	// Prompt blocks until the sidecar completes the ACP turn. During processing,
	// SessionUpdate callbacks stream text to the HTTP response via clientImpl.
	promptResp, err := acp.SendRequest[acp.PromptResponse](acpConn, acpCtx, acp.AgentMethodSessionPrompt, acp.PromptRequest{
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

	clientImpl.writeAndFlush(fmt.Sprintf("\n[stop_reason] %s\n", promptResp.StopReason))
}
