// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	aguitypes "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
	acp "github.com/coder/acp-go-sdk"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/version"
)

// ChatRequest is the AG-UI payload accepted by the chat endpoint.
type ChatRequest = aguitypes.RunAgentInput

// ChatHandler manages the AI gateway requests. Incoming AG-UI RunAgentInput
// payloads are translated into ACP prompts against a sidecar agent, and the
// resulting ACP notifications are streamed back to the caller as AG-UI SSE
// events.
type ChatHandler struct {
	Logger             *zap.Logger
	QueryService       *querysvc.QueryService
	sidecarWSURL       string
	maxRequestBodySize int64
}

// NewChatHandler wires the chat endpoint against a sidecar WebSocket URL.
// QueryService may be nil in tests that do not exercise contextual tooling.
func NewChatHandler(logger *zap.Logger, queryService *querysvc.QueryService, sidecarWSURL string, maxRequestBodySize int64) *ChatHandler {
	return &ChatHandler{
		Logger:             logger,
		QueryService:       queryService,
		sidecarWSURL:       sidecarWSURL,
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

	prompt, err := latestUserMessageText(req.Messages)
	if err != nil {
		http.Error(w, "prompt is required", http.StatusBadRequest)
		return
	}

	promptBlocks := []acp.ContentBlock{acp.TextBlock(prompt)}
	for _, ctxText := range contextTextEntries(req.Context) {
		promptBlocks = append(promptBlocks, acp.TextBlock(ctxText))
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

	clientImpl := newStreamingClient(ctx, w, req.RunID)

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

	sess, err := acpConn.NewSession(acpCtx, acp.NewSessionRequest{
		Cwd:        "/",
		McpServers: []acp.McpServer{},
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("Error creating session: %v", err), http.StatusBadGateway)
		return
	}

	// Publish the frontend-provided tools snapshot so the agent can retrieve
	// them via the list_contextual_tools MCP tool.
	if h.QueryService != nil {
		h.QueryService.SetContextualToolsForSession(string(sess.SessionId), encodeToolsAsRaw(req.Tools))
	}

	// Set streaming headers just before Prompt: SessionUpdate callbacks start
	// firing SSE frames from here on, and headers must be committed before
	// the first write. Earlier errors (dial/initialize/new_session) use
	// http.Error with a standard status code since no streaming contract has
	// been established yet.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	clientImpl.startRun()

	// Prompt blocks until the sidecar completes the ACP turn. The acp-go-sdk
	// drains pending SessionUpdate notifications before the call returns, so
	// by the time we reach finishRun() all streamed content has been written.
	promptResp, err := acpConn.Prompt(acpCtx, acp.PromptRequest{
		SessionId: sess.SessionId,
		Prompt:    promptBlocks,
	})
	if err != nil {
		clientImpl.failRun(fmt.Sprintf("Error starting prompt: %v", err))
		return
	}

	clientImpl.finishRun(string(promptResp.StopReason))
}
