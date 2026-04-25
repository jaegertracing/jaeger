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
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/version"
)

// contextualMCPServerName is the ACP-visible name for the per-turn MCP
// server that surfaces the frontend-provided AG-UI tool snapshot.
const contextualMCPServerName = "jaeger-ai-contextual"

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
// the handler uses it to build the absolute URL the sidecar will dial
// back for per-turn contextual MCP tools.
func NewChatHandler(logger *zap.Logger, ctxTools *ContextualToolsStore, sidecarWSURL, basePath string, maxRequestBodySize int64) *ChatHandler {
	return &ChatHandler{
		Logger:             logger,
		ctxTools:           ctxTools,
		sidecarWSURL:       sidecarWSURL,
		basePath:           basePath,
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
	// Build an ACP client-side connection over the websocket adapter.
	acpConn := acp.NewClientSideConnection(clientImpl, adapter, adapter)

	_, err = acpConn.Initialize(acpCtx, acp.InitializeRequest{
		ProtocolVersion: acp.ProtocolVersionNumber,
		ClientCapabilities: acp.ClientCapabilities{
			Fs:       acp.FileSystemCapability{ReadTextFile: false, WriteTextFile: false},
			Terminal: false,
		},
		ClientInfo: &acp.Implementation{
			Name:    "jaeger-ai-gateway",
			Version: version.Get().GitVersion,
		},
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("Error initializing agent: %v", err), http.StatusBadGateway)
		return
	}

	// Mint a per-turn id that ties this chat request's AG-UI tool snapshot
	// to the contextual MCP endpoint the sidecar will read from. The URL is
	// constructed from the inbound request so deployments that expose
	// jaeger-query on one hostname but internally resolve it under another
	// still send the sidecar back to where the gateway actually listens.
	contextualMCPID := uuid.NewString()
	contextualMCPURL := h.buildContextualMCPURL(r, contextualMCPID)

	sess, err := acpConn.NewSession(acpCtx, acp.NewSessionRequest{
		Cwd: "/",
		McpServers: []acp.McpServer{{
			Http: &acp.McpServerHttpInline{
				Name:    contextualMCPServerName,
				Type:    "http",
				Url:     contextualMCPURL,
				Headers: []acp.HttpHeader{},
			},
		}},
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("Error creating session: %v", err), http.StatusBadGateway)
		return
	}

	// Set streaming headers just before Prompt(), since SessionUpdate callbacks
	// may start writing to the response during this call.
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")

	// TODO: wire the frontend-provided AG-UI tools snapshot into h.ctxTools
	// (SetForContextualMCPID before Prompt, deferred DeleteForContextualMCPID
	// after) once the chat request carries a Tools field. Until then the
	// store is populated only via code paths added in the AG-UI gateway PR.

	// Prompt blocks until the sidecar completes the ACP turn. During processing,
	// SessionUpdate callbacks stream text to the HTTP response via clientImpl.
	promptResp, err := acpConn.Prompt(acpCtx, acp.PromptRequest{
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

// buildContextualMCPURL reconstructs the absolute URL at which the gateway
// serves the per-turn contextual MCP endpoint. The sidecar dials this URL
// after receiving it in NewSessionRequest.McpServers, so it must be
// reachable from the sidecar.
func (h *ChatHandler) buildContextualMCPURL(r *http.Request, id string) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	prefix := strings.TrimSuffix(h.basePath, "/")
	return scheme + "://" + r.Host + prefix + "/api/ai/mcp/" + id
}
