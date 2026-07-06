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
	"github.com/google/uuid"
	oteltrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/mcptools"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/telemetry"
	"github.com/jaegertracing/jaeger/internal/telemetry/otelsemconv"
	"github.com/jaegertracing/jaeger/internal/tenancy"
	"github.com/jaegertracing/jaeger/internal/version"
)

// Handler is the entry point for the jaeger-query AI gateway. It owns the
// per-turn contextual tools store, the session-stream registry, and the chat
// handler, and registers them on the caller-provided mux (see RegisterRoutes in
// routes.go).
//
// Callers construct a Handler once (in jaegerquery's Start path), then call
// RegisterRoutes when wiring the HTTP mux. This mirrors the APIHandler /
// HTTPGateway pattern used by sibling jaeger-query subsystems and keeps all
// AI dependencies inside the jaegerai package.
type Handler struct {
	logger *zap.Logger
	// store and streams are two per-session registries that are separate only
	// during this transition, because they're keyed differently: store by the
	// ACP session id (read by the ext-method tool-call dispatch), streams by the
	// gateway-minted UUID in the session-scoped MCP URL — and there is no bridge
	// between the two ids. Once UI tools are served over the MCP endpoint and the
	// ext-method path is retired, they collapse into one session container keyed
	// by the UUID.
	store              *ContextualToolsStore
	streams            *sessionStreams
	agentURL           string
	basePath           string
	maxRequestBodySize int64
	// mcpHandler serves the session-scoped MCP endpoint. Non-nil only when the
	// operator enabled MCP (HandlerParams.EnableMCP); otherwise the endpoint is
	// not mounted and the gateway advertises AI chat only.
	mcpHandler http.Handler
}

// HandlerParams carries the dependencies for the AI gateway Handler. Grouping
// them in a struct keeps the constructor readable as the gateway gains MCP
// wiring (query service, tenancy, telemetry) on top of the chat parameters.
type HandlerParams struct {
	Logger             *zap.Logger
	AgentURL           string
	BasePath           string
	MaxRequestBodySize int64
	// EnableMCP mounts the session-scoped telemetry MCP endpoint. When false,
	// only the chat endpoint is registered.
	EnableMCP    bool
	QueryService *querysvc.QueryService
	TenancyMgr   *tenancy.Manager
	Telset       telemetry.Settings
}

// NewHandler constructs a jaegerai.Handler with a freshly-allocated
// ContextualToolsStore and sessionStreams. basePath is normalized once so the
// registered mux patterns use a single canonical prefix. When p.EnableMCP is
// set, the session-scoped MCP handler is built from the supplied query service,
// tenancy manager, and telemetry settings.
func NewHandler(p HandlerParams) *Handler {
	basePath := normalizeBasePath(p.BasePath)
	h := &Handler{
		logger:             p.Logger,
		store:              NewContextualToolsStore(),
		streams:            newSessionStreams(),
		agentURL:           p.AgentURL,
		basePath:           basePath,
		maxRequestBodySize: p.MaxRequestBodySize,
	}
	if p.EnableMCP {
		mcpHandler := mcptools.NewHandler(p.Telset, p.QueryService, p.TenancyMgr, mcptools.DefaultConfig())
		h.mcpHandler = &mcpSessionHandler{
			telemetryHandler: mcpHandler,
			streams:          h.streams,
			basePath:         basePath,
			logger:           p.Logger,
		}
	}
	return h
}

// ChatRequest is the AG-UI payload accepted by the chat endpoint. It is the
// AG-UI RunAgentInput shape — messages, tools, context, thread/run ids — so
// the gateway can be addressed by stock AG-UI clients without translation.
type ChatRequest = aguitypes.RunAgentInput

// ChatHandler manages the AI gateway requests. Incoming AG-UI RunAgentInput
// payloads are translated into ACP prompts against a sidecar agent, and the
// resulting ACP notifications are streamed back to the caller as AG-UI SSE
// events.
type ChatHandler struct {
	Logger   *zap.Logger
	ctxTools *ContextualToolsStore
	// streams registers this turn's SSE streaming client under a session id so
	// the session-scoped MCP endpoint can confirm the id belongs to an active
	// turn. May be nil in tests that do not exercise session registration.
	streams            *sessionStreams
	sidecarWSURL       string
	basePath           string
	maxRequestBodySize int64
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

	// Register this turn's stream under a freshly-minted session id so the
	// session-scoped MCP endpoint (/api/ai/mcp/<id>/) can confirm the id
	// belongs to an active turn. The id is minted here rather than reusing the
	// ACP session id because the endpoint URL must be constructible before
	// session/new returns; announcing that URL to the sidecar is a follow-up.
	if h.streams != nil {
		mcpSessionID := uuid.NewString()
		h.streams.set(mcpSessionID, clientImpl)
		defer h.streams.delete(mcpSessionID)
	}

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

	// Enrich the existing HTTP server span (created by otelhttp around this
	// handler) with GenAI attributes identifying the sidecar agent handling
	// this turn. No new span: the agent's own name/version only becomes known
	// after this response, so it can't be set as a span-start attribute, and
	// a dedicated child span would just duplicate what the HTTP span already
	// records for timing/status.
	span := oteltrace.SpanFromContext(ctx)
	span.SetAttributes(otelsemconv.GenAIOperationNameInvokeAgent)
	if init.AgentInfo != nil {
		span.SetAttributes(otelsemconv.GenAIAgentName(init.AgentInfo.Name))
		if init.AgentInfo.Version != "" {
			span.SetAttributes(otelsemconv.GenAIAgentVersion(init.AgentInfo.Version))
		}
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
	span.SetAttributes(otelsemconv.GenAIConversationID(string(sess.SessionId)))

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
	//
	// Inject the active trace context into _meta so a sidecar that extracts
	// it (SEP-414 style, same as the MCP tool-call boundary) parents its own
	// agentic-loop spans under this request's span, joining what would
	// otherwise be two disconnected traces into one.
	promptResp, err := acp.SendRequest[acp.PromptResponse](acpConn, acpCtx, acp.AgentMethodSessionPrompt, acp.PromptRequest{
		SessionId: sess.SessionId,
		Prompt:    promptBlocks,
		Meta:      injectTraceContextIntoMeta(ctx, nil),
	})
	if err != nil {
		clientImpl.failRun(fmt.Sprintf("Error starting prompt: %v", err))
		return
	}

	clientImpl.finishRun(string(promptResp.StopReason))
}
