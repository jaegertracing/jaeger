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
	modelContextLimit  int
}

// NewChatHandler wires the chat endpoint against a sidecar WebSocket URL.
// ctxTools may be nil in tests that do not exercise contextual tooling.
// basePath is the jaeger-query base path; it is normalized once and kept
// on the handler for consistency with other route handlers in this
// package (APIHandler, static_handler) even though ServeHTTP does not
// currently read it.
func NewChatHandler(logger *zap.Logger, ctxTools *ContextualToolsStore, sidecarWSURL, basePath string, maxRequestBodySize int64, modelContextLimit int) *ChatHandler {
	return &ChatHandler{
		Logger:             logger,
		ctxTools:           ctxTools,
		sidecarWSURL:       sidecarWSURL,
		basePath:           normalizeBasePath(basePath),
		maxRequestBodySize: maxRequestBodySize,
		modelContextLimit:  modelContextLimit,
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
		http.Error(w, "messages must include a user message with text content", http.StatusBadRequest)
		return
	}

	// Estimate total tokens of prompt and context values, and prune if needed.
	totalEstimatedTokens := estimateTokens(prompt)
	for _, ctxText := range contextTextEntries(req.Context) {
		totalEstimatedTokens += estimateTokens(ctxText)
	}

	limit := h.modelContextLimit
	if limit <= 0 {
		limit = DefaultAIModelContextLimit
	}

	if totalEstimatedTokens > limit {
		// Attempt to prune trace context values to fit within the limit.
		prunedContext := make([]aguitypes.Context, len(req.Context))
		copy(prunedContext, req.Context)
		anyPruned := false
		for i, entry := range prunedContext {
			if newVal, pruned := tryPruneTraceContext(entry.Value); pruned {
				prunedContext[i].Value = newVal
				anyPruned = true
			}
		}
		if anyPruned {
			req.Context = prunedContext
			// Re-estimate tokens
			totalEstimatedTokens = estimateTokens(prompt)
			for _, ctxText := range contextTextEntries(req.Context) {
				totalEstimatedTokens += estimateTokens(ctxText)
			}
		}

		if totalEstimatedTokens > limit {
			// Graceful Degradation: return a clear error warning to the UI
			http.Error(w, "Request too large for local model. Please filter or prune spans first.", http.StatusBadRequest)
			return
		}
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

	// Build two snapshots from the same input:
	//   - prefixedTools (Meta wire shape): names are UIToolPrefix-namespaced
	//     so the sidecar registers them with the LLM under the prefix, which
	//     guarantees no collision with built-in Jaeger MCP tool names.
	//   - rawTools (ContextualToolsStore): the original frontend names, so
	//     handleJaegerToolCall can validate dispatches against the unprefixed
	//     name it gets after stripping. Storing prefixed bytes here would
	//     force every consumer to know about the transport-level prefix.
	prefixedTools := prefixContextualTools(req.Tools)
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

	clientImpl := newStreamingClient(ctx, w, req.ThreadID, req.RunID)
	// Build the ACP connection ourselves so the inbound dispatcher can
	// route both standard ACP methods (session/update etc.) and our
	// extension method (ExtMethodJaegerToolCall) — the SDK's
	// NewClientSideConnection has a hardcoded dispatcher that returns
	// MethodNotFound for any extension method we add.
	acpConn := acp.NewConnection(newDispatcher(clientImpl, h.ctxTools, h.Logger), adapter, adapter)

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
		McpServers: []acp.McpServer{},
	}
	if len(prefixedTools) > 0 {
		newSessionReq.Meta = map[string]any{
			ContextualToolsMetaKey: map[string]any{
				"tools": prefixedTools,
			},
		}
	}
	sess, err := acp.SendRequest[acp.NewSessionResponse](acpConn, acpCtx, acp.AgentMethodSessionNew, newSessionReq)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error creating session: %v", err), http.StatusBadGateway)
		return
	}

	defer closeACPSession(ctx, acpConn, init.AgentCapabilities, sess.SessionId, h.Logger)

	// Publish the unprefixed snapshot under the assigned ACP session id so
	// handleJaegerToolCall can confirm dispatches by their post-strip
	// (frontend) name. Cleared at turn end regardless of success.
	if h.ctxTools != nil && len(rawTools) > 0 {
		sessionID := string(sess.SessionId)
		h.ctxTools.SetForSession(sessionID, rawTools)
		defer h.ctxTools.DeleteForSession(sessionID)
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
