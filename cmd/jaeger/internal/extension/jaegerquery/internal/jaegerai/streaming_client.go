// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	aguievents "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	aguisse "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/encoding/sse"
	"github.com/coder/acp-go-sdk"
	"go.uber.org/zap"
)

// streamingClientIDSeq is a process-wide monotonic counter appended to the
// time-derived stem when startRun allocates IDs. Even on systems with
// coarse clock resolution (some VMs, older Windows) or two streamingClients
// constructed within the same nanosecond, the counter guarantees no two
// runs in this process share a stem. The combination (nanos-seq) is also
// time-sortable across processes, which keeps logs from a single Jaeger
// instance well-ordered.
var streamingClientIDSeq atomic.Uint64

var _ acp.Client = (*streamingClient)(nil)

// streamingClient implements acp.Client and translates ACP session updates
// into AG-UI SSE events written to the HTTP response.
//
// Events are constructed via the typed AG-UI SDK constructors so that
// schema-required fields (toolCallName, messageId, etc.) are positional —
// forgetting one is a compile error rather than a runtime ZodError on the
// frontend. Framing is delegated to the SDK's SSEWriter.
//
// All mutable fields (closed, threadID, runID, messageID, textOpen) are
// guarded by mu. The ACP SDK may invoke SessionUpdate on a goroutine other
// than the one driving Prompt, and lifecycle calls (startRun, finishRun,
// failRun) come from the chat handler — every entry point that touches
// state or writes a frame must acquire the lock first.
type streamingClient struct {
	requestCtx context.Context
	w          http.ResponseWriter
	sse        *aguisse.SSEWriter
	mu         sync.Mutex
	closed     bool
	threadID   string
	runID      string
	messageID  string
	textOpen   bool
}

func newStreamingClient(ctx context.Context, w http.ResponseWriter, threadID, runID string) *streamingClient {
	return &streamingClient{
		requestCtx: ctx,
		w:          w,
		sse:        aguisse.NewSSEWriter(),
		threadID:   threadID,
		runID:      runID,
	}
}

// emit writes a typed AG-UI event as a single SSE frame. The SDK's
// SSEWriter handles JSON encoding, newline escaping, and flushing.
// Must be called with c.mu held; on context cancellation or write error
// the client is marked closed so subsequent emissions are dropped.
func (c *streamingClient) emit(event aguievents.Event) {
	if c.closed {
		return
	}
	if c.requestCtx != nil {
		select {
		case <-c.requestCtx.Done():
			c.closed = true
			return
		default:
		}
	}
	if err := c.sse.WriteEvent(c.requestCtx, c.w, event); err != nil {
		c.closed = true
	}
}

// startRun emits RUN_STARTED and allocates a message id for the assistant
// text stream. ThreadID and RunID are preserved if the caller provided them
// via the AG-UI RunAgentInput payload; otherwise time-derived defaults fill
// in so the frontend always sees stable identifiers per run.
func (c *streamingClient) startRun() {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Allocate one stem per startRun and derive all three IDs from it.
	// The stem is "<nanos>-<seq>": the nanosecond is time-sortable across
	// processes, and the process-wide atomic counter guarantees no two
	// runs in this process collide even on coarse-clock systems where
	// two startRun calls might land in the same nanosecond. Sharing the
	// stem also makes the trio visibly correlated in logs/traces — a
	// thread-N-K, run-N-K, msg-N-K triplet tells a debugger at a glance
	// they came from the same run startup.
	stem := fmt.Sprintf("%d-%d", time.Now().UnixNano(), streamingClientIDSeq.Add(1))
	if c.threadID == "" {
		c.threadID = "thread-" + stem
	}
	if c.runID == "" {
		c.runID = "run-" + stem
	}
	if c.messageID == "" {
		c.messageID = "msg-" + stem
	}
	c.emit(aguievents.NewRunStartedEvent(c.threadID, c.runID))
}

// finishRun closes any open text message and emits RUN_FINISHED. The
// AG-UI schema has no top-level stopReason field, so the sidecar's stop
// reason is forwarded via the schema-supported `result` payload as
// `{"stopReason": "<reason>"}`. Empty stop reasons are omitted so the
// emitted event stays compact.
func (c *streamingClient) finishRun(stopReason string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.textOpen {
		c.emit(aguievents.NewTextMessageEndEvent(c.messageID))
		c.textOpen = false
	}
	if stopReason != "" {
		c.emit(aguievents.NewRunFinishedEventWithOptions(
			c.threadID, c.runID,
			aguievents.WithResult(map[string]any{"stopReason": stopReason}),
		))
		return
	}
	c.emit(aguievents.NewRunFinishedEvent(c.threadID, c.runID))
}

// failRun emits RUN_ERROR with the supplied message. It is safe to call from
// any handler error path; an open text message is closed first so frontends
// can finalize rendering before reporting the failure.
func (c *streamingClient) failRun(message string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.textOpen {
		c.emit(aguievents.NewTextMessageEndEvent(c.messageID))
		c.textOpen = false
	}
	c.emit(aguievents.NewRunErrorEvent(message, aguievents.WithRunID(c.runID)))
}

// ensureTextStart emits TEXT_MESSAGE_START the first time streamed assistant
// text is observed. Subsequent chunks are emitted as TEXT_MESSAGE_CONTENT.
// A messageID is allocated lazily here as well as in startRun so an
// out-of-order SessionUpdate (no preceding startRun) still produces a
// schema-valid event — the SDK rejects events with empty required fields
// at encode time. Must be called with c.mu held.
func (c *streamingClient) ensureTextStart() {
	if c.textOpen {
		return
	}
	if c.messageID == "" {
		c.messageID = fmt.Sprintf("msg-%d", time.Now().UnixNano())
	}
	c.emit(aguievents.NewTextMessageStartEvent(c.messageID))
	c.textOpen = true
}

// EmitContextualToolCall fires the TOOL_CALL_START / TOOL_CALL_ARGS /
// TOOL_CALL_END lifecycle for a UI tool the agent invoked via the
// session-scoped MCP endpoint, writing the events onto the browser SSE stream so
// the frontend executes the tool. It acquires c.mu like the other entry points,
// so it is safe to call concurrently with the ACP SessionUpdate path.
func (c *streamingClient) EmitContextualToolCall(toolCallID, toolName string, rawArgs any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.emit(aguievents.NewToolCallStartEvent(toolCallID, toolName))
	if rawArgs != nil {
		c.emit(aguievents.NewToolCallArgsEvent(toolCallID, marshalToolArgsDelta(rawArgs)))
	}
	c.emit(aguievents.NewToolCallEndEvent(toolCallID))
}

// RequestPermission auto-accepts tool calls by selecting the first
// allow-flavoured option the agent offers. The gateway curates which tools
// an agent can invoke through two pre-approved channels:
//   - MCP tools the operator wires up at deploy time (read-only telemetry
//     queries against Jaeger).
//   - Contextual tools the frontend declares for the chat turn (browser-
//     side AG-UI tools the user is implicitly authorising by sending the
//     prompt).
//
// Filesystem and terminal capabilities are disabled in Initialize, so a
// well-behaved agent will not have other tools to ask about; if one does
// ask, we still accept because the tool itself was already curated upstream.
// AG-UI has no permission-request event shape to bubble a per-call prompt
// up to the browser, so deferring would be useless.
//
// The previous "always cancel" policy was incompatible with agents like the
// Claude Agent SDK, which routes every tool call through canUseTool / a
// session/request_permission RPC; the cancelled outcome aborted every call
// as "Tool use aborted". The Gemini sidecar never hit this because Gemini's
// tool path does not issue ACP permission requests.
//
// If the agent presents no allow-kind option (e.g. only reject choices),
// we fall through to cancelled so a misconfigured agent cannot be coerced
// into a forbidden action.
func (*streamingClient) RequestPermission(_ context.Context, req acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	for _, opt := range req.Options {
		if opt.Kind == acp.PermissionOptionKindAllowOnce || opt.Kind == acp.PermissionOptionKindAllowAlways {
			return acp.RequestPermissionResponse{
				Outcome: acp.RequestPermissionOutcome{
					Selected: &acp.RequestPermissionOutcomeSelected{OptionId: opt.OptionId},
				},
			}, nil
		}
	}
	return acp.RequestPermissionResponse{
		Outcome: acp.RequestPermissionOutcome{
			Cancelled: &acp.RequestPermissionOutcomeCancelled{},
		},
	}, nil
}

func (c *streamingClient) SessionUpdate(_ context.Context, n acp.SessionNotification) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	u := n.Update
	if u.AgentMessageChunk != nil {
		content := u.AgentMessageChunk.Content
		// AG-UI's TextMessageContentEvent forbids empty deltas and the SDK
		// encoder rejects them at write time, marking the SSE stream closed
		// and stranding the browser bubble half-rendered. ACP, however,
		// allows agents to emit text-block lifecycle/framing chunks whose
		// `text` is "" or whitespace-only (Claude's stream maps
		// content_block_start → empty-text chunk this way). Skip those at
		// the boundary so the AG-UI invariant START → CONTENT+ → END holds
		// and a noisy upstream agent doesn't take down the run. Note we
		// emit the original Text (not the trimmed value) — leading/trailing
		// whitespace inside a multi-chunk message is meaningful glue
		// between deltas, only fully-blank chunks are skipped.
		if content.Text != nil && strings.TrimSpace(content.Text.Text) != "" {
			c.ensureTextStart()
			c.emit(aguievents.NewTextMessageContentEvent(c.messageID, content.Text.Text))
		}
	}
	if u.ToolCall != nil {
		// The sidecar populates ACP Title with the tool identifier (prefixed
		// with UIToolPrefix for contextual tools). The prefix is stripped
		// here so the frontend sees the same name it registered the tool
		// under, regardless of the wire-level namespace.
		c.emit(aguievents.NewToolCallStartEvent(
			string(u.ToolCall.ToolCallId),
			stripUIToolPrefix(u.ToolCall.Title),
		))
		if u.ToolCall.RawInput != nil {
			c.emit(aguievents.NewToolCallArgsEvent(
				string(u.ToolCall.ToolCallId),
				marshalToolArgsDelta(u.ToolCall.RawInput),
			))
		}
	}
	if u.ToolCallUpdate != nil {
		if u.ToolCallUpdate.RawInput != nil {
			c.emit(aguievents.NewToolCallArgsEvent(
				string(u.ToolCallUpdate.ToolCallId),
				marshalToolArgsDelta(u.ToolCallUpdate.RawInput),
			))
		}
		if u.ToolCallUpdate.RawOutput != nil {
			c.emit(aguievents.NewToolCallResultEvent(
				toolResultMessageID(u.ToolCallUpdate.ToolCallId),
				string(u.ToolCallUpdate.ToolCallId),
				flattenToolResultContent(u.ToolCallUpdate.RawOutput),
			))
		}
		status := valueOrUnknown(u.ToolCallUpdate.Status)
		if status == string(acp.ToolCallStatusCompleted) || status == string(acp.ToolCallStatusFailed) {
			c.emit(aguievents.NewToolCallEndEvent(string(u.ToolCallUpdate.ToolCallId)))
		}
	}
	return nil
}

// The methods below implement acp.Client operations that the Jaeger gateway
// does not support (filesystem and terminal). They return errors because the
// client advertises these capabilities as disabled during Initialize.

var errNotSupported = errors.New("operation not supported by jaeger-ai-gateway")

func (*streamingClient) WriteTextFile(context.Context, acp.WriteTextFileRequest) (acp.WriteTextFileResponse, error) {
	return acp.WriteTextFileResponse{}, errNotSupported
}

func (*streamingClient) ReadTextFile(context.Context, acp.ReadTextFileRequest) (acp.ReadTextFileResponse, error) {
	return acp.ReadTextFileResponse{}, errNotSupported
}

func (*streamingClient) CreateTerminal(context.Context, acp.CreateTerminalRequest) (acp.CreateTerminalResponse, error) {
	return acp.CreateTerminalResponse{}, errNotSupported
}

func (*streamingClient) KillTerminal(context.Context, acp.KillTerminalRequest) (acp.KillTerminalResponse, error) {
	return acp.KillTerminalResponse{}, errNotSupported
}

func (*streamingClient) ReleaseTerminal(context.Context, acp.ReleaseTerminalRequest) (acp.ReleaseTerminalResponse, error) {
	return acp.ReleaseTerminalResponse{}, errNotSupported
}

func (*streamingClient) TerminalOutput(context.Context, acp.TerminalOutputRequest) (acp.TerminalOutputResponse, error) {
	return acp.TerminalOutputResponse{}, errNotSupported
}

func (*streamingClient) WaitForTerminalExit(context.Context, acp.WaitForTerminalExitRequest) (acp.WaitForTerminalExitResponse, error) {
	return acp.WaitForTerminalExitResponse{}, errNotSupported
}

// closeACPSession is a best-effort cleanup hook: it tells the agent to
// release the session before the gateway tears down the WebSocket. Meant
// to be invoked via `defer` in ServeHTTP, after defer adapter.Close so it
// fires first (LIFO) while the connection is still open.
//
// The capability gate ensures we never call session/close against agents
// that don't advertise support — they would respond with MethodNotFound.
// WithoutCancel detaches the request context's cancellation so cleanup
// still runs after the client disconnects mid-stream, while preserving
// values such as tracing. The 5s deadline keeps a stuck agent from
// hanging the goroutine indefinitely. Errors are Debug-logged because the
// HTTP response has already been streamed by the time this runs.
func closeACPSession(ctx context.Context, conn *acp.Connection, caps acp.AgentCapabilities, sessionID acp.SessionId, logger *zap.Logger) {
	if caps.SessionCapabilities.Close == nil {
		return
	}
	closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if _, err := acp.SendRequest[acp.CloseSessionResponse](conn, closeCtx, acp.AgentMethodSessionClose, acp.CloseSessionRequest{
		SessionId: sessionID,
	}); err != nil {
		logger.Debug("session/close failed", zap.String("session_id", string(sessionID)), zap.Error(err))
	}
}
