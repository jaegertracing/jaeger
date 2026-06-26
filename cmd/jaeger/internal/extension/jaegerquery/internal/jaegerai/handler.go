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

	"github.com/jaegertracing/jaeger/internal/version"
)

// ChatRequest is the AG-UI payload accepted by the chat endpoint. It is the
// AG-UI RunAgentInput shape — messages, tools, context, thread/run ids — so
// the gateway can be addressed by stock AG-UI clients without translation.
type ChatRequest = aguitypes.RunAgentInput

// ChatHandler manages the AI gateway requests. Incoming AG-UI RunAgentInput
// payloads are translated into ACP prompts against a sidecar agent, and the
// resulting ACP notifications are streamed back to the caller as AG-UI SSE
// events.
type ChatHandler struct {
	Logger             *zap.Logger
	ctxTools           *ContextualToolsStore
	sidecarWSURL       string
	basePath           string
	maxRequestBodySize int64
	// streams is the registry the MCP proxy reads to find the SSE
	// stream for a UI-tool dispatch. Optional — when nil, ServeHTTP
	// simply skips the session/stream bookkeeping and the MCP proxy
	// returns "no active chat session" for any tools/call. Tests
	// that don't exercise the MCP proxy can leave it unset.
	streams *SessionStreams
	// mcpProxy is the MCP-over-ACP target the dispatcher routes mcp/*
	// requests to when the agent advertises mcp_capabilities.acp.
	// Optional — when nil the three mcp/* methods land on
	// MethodNotFound, signalling to the agent that the gateway is
	// not an MCP-over-ACP endpoint for this chat.
	mcpProxy *MCPProxy
}

// NewChatHandler wires the chat endpoint against a sidecar WebSocket URL.
// ctxTools may be nil in tests that do not exercise contextual tooling.
// basePath is the jaeger-query base path; it is normalized once and kept
// on the handler for consistency with other route handlers in this
// package (APIHandler, static_handler) even though ServeHTTP does not
// currently read it.
func NewChatHandler(logger *zap.Logger, ctxTools *ContextualToolsStore, sidecarWSURL, basePath string, maxRequestBodySize int64) *ChatHandler {
	return &ChatHandler{
		Logger:             logger,
		ctxTools:           ctxTools,
		sidecarWSURL:       sidecarWSURL,
		basePath:           normalizeBasePath(basePath),
		maxRequestBodySize: maxRequestBodySize,
	}
}

// withSessionStreams attaches the per-process SessionStreams registry so
// the chat handler can register the live streaming client under the ACP
// session id once it's allocated, letting the MCP proxy find the SSE
// stream for UI-tool dispatch. Package-private builder rather than a
// constructor arg so existing test call sites — which never need the
// MCP proxy path — don't have to thread `nil` through.
func (h *ChatHandler) withSessionStreams(streams *SessionStreams) *ChatHandler {
	h.streams = streams
	return h
}

// withMCPProxy attaches the shared MCPProxy so the dispatcher can route
// mcp/connect, mcp/disconnect, and mcp/message into the proxy's
// HandleConnect / HandleDisconnect / HandleMessage methods. Builder
// rather than constructor arg for the same reason as withSessionStreams
// — keeps existing test call sites untouched.
func (h *ChatHandler) withMCPProxy(proxy *MCPProxy) *ChatHandler {
	h.mcpProxy = proxy
	return h
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
		http.Error(w, "messages must include a user message with text content", http.StatusBadRequest)
		return
	}

	// Reject blank tool names up front. Empty or whitespace-only names
	// would prefix to "ui_" (or "ui_   "), which the dispatcher later
	// rejects as InvalidParams and which would also pollute both the Meta
	// payload and the ContextualToolsStore snapshot with unusable entries.
	// Failing fast here keeps both gateway-side data structures clean and
	// surfaces frontend bugs as a 400 rather than a silent mid-turn error.
	if err := validateContextualToolNames(req.Tools); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	promptBlocks := []acp.ContentBlock{acp.TextBlock(prompt)}
	for _, ctxText := range contextTextEntries(req.Context) {
		promptBlocks = append(promptBlocks, acp.TextBlock(ctxText))
	}

	// rawTools is the per-turn snapshot the MCPProxy reads in
	// listToolsForSession / callToolForSession to advertise and route
	// UI tools. Names are stored unprefixed (matching what the frontend
	// sent); the UIToolPrefix is applied on the MCP wire by the proxy
	// when emitting tools/list and stripped back before lookup.
	rawTools := encodeToolsAsRaw(req.Tools)

	ctx := r.Context()
	acpCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	adapter, err := DialWsAdapter(acpCtx, h.sidecarWSURL, h.Logger)
	if err != nil {
		http.Error(w, "Failed to connect to agent backend", http.StatusBadGateway)
		return
	}
	defer adapter.Close()

	// Mint the per-turn UUID up front so it can be embedded in the
	// announced mcpServers entry (the agent receives the URL before
	// the gateway knows the sidecar's session id). The mapping
	// uuid → sessionId is registered after session/new returns and
	// torn down in defer on turn end.
	var sessionUUID string
	if h.mcpProxy != nil {
		sessionUUID = NewSessionUUID()
		defer h.mcpProxy.UnregisterUUID(sessionUUID)
	}

	clientImpl := newStreamingClient(ctx, w, req.ThreadID, req.RunID)
	// Build the ACP connection ourselves so the inbound dispatcher can
	// route both standard ACP methods (session/update etc.) and the
	// MCP-over-ACP method family — the SDK's NewClientSideConnection
	// has a hardcoded dispatcher that returns MethodNotFound for any
	// extension method we add.
	acpConn := acp.NewConnection(
		newDispatcher(clientImpl, h.mcpProxy),
		adapter, adapter,
	)

	init, err := acp.SendRequest[acp.InitializeResponse](acpConn, acpCtx, acp.AgentMethodInitialize, acp.InitializeRequest{
		ProtocolVersion: acp.ProtocolVersionNumber,
		ClientCapabilities: acp.ClientCapabilities{
			Fs:       acp.FileSystemCapabilities{ReadTextFile: false, WriteTextFile: false},
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

	newSessionReq := acp.NewSessionRequest{
		// "/" is a placeholder: the gateway advertises no fs capability in
		// Initialize, so Cwd is never resolved against a real filesystem.
		// ACP requires the field to be non-empty, hence this constant.
		Cwd:        "/",
		McpServers: announceMCPServers(init, h.mcpProxy, sessionUUID),
	}
	sess, err := acp.SendRequest[acp.NewSessionResponse](acpConn, acpCtx, acp.AgentMethodSessionNew, newSessionReq)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error creating session: %v", err), http.StatusBadGateway)
		return
	}

	defer closeACPSession(ctx, acpConn, init.AgentCapabilities, sess.SessionId, h.Logger)

	// Register the uuid → sessionID mapping so subsequent dials from
	// the sidecar — HTTP via the announced URL, or ACP via mcp/connect
	// with the same uuid as acpId — resolve to this turn's session.
	if h.mcpProxy != nil {
		h.mcpProxy.RegisterUUIDForSession(sessionUUID, string(sess.SessionId))
	}

	// Publish the per-session UI-tool snapshot so the MCP proxy can
	// expose them via tools/list and dispatch them via tools/call.
	// Cleared at turn end regardless of success.
	if h.ctxTools != nil && len(rawTools) > 0 {
		sessionID := string(sess.SessionId)
		h.ctxTools.SetForSession(sessionID, rawTools)
		defer h.ctxTools.DeleteForSession(sessionID)
	}

	// Register the live streaming client under the ACP session id so the
	// MCP proxy can find it when an agent invokes a UI tool. Optional —
	// when h.streams is nil (tests, deployments without the MCP route
	// wired in) we skip both Set and Delete, and the proxy simply returns
	// "no active chat session" for any tools/call. The MCP proxy
	// resolves the uuid in the announced URL back to this session id
	// via the uuidToSession map before consulting streams.
	if h.streams != nil {
		sessionID := string(sess.SessionId)
		h.streams.Set(sessionID, clientImpl)
		defer h.streams.Delete(sessionID)
	}

	// Set streaming headers just before Prompt: SessionUpdate callbacks
	// start firing SSE frames from here on, and headers must be committed
	// before the first write. Earlier errors (decode/dial/initialize/
	// new_session) use http.Error with a standard status code since no
	// streaming contract has been established yet.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")

	clientImpl.startRun()

	// Prompt blocks until the sidecar completes the ACP turn. The
	// acp-go-sdk drains pending SessionUpdate notifications before the
	// call returns, so by the time we reach finishRun() all streamed
	// content has been written.
	promptResp, err := acp.SendRequest[acp.PromptResponse](acpConn, acpCtx, acp.AgentMethodSessionPrompt, acp.PromptRequest{
		SessionId: sess.SessionId,
		Prompt:    promptBlocks,
	})
	if err != nil {
		clientImpl.failRun(fmt.Sprintf("Error starting prompt: %v", err))
		return
	}

	clientImpl.finishRun(string(promptResp.StopReason))
}
