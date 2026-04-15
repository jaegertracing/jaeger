// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/coder/acp-go-sdk"
)

var _ acp.Client = (*streamingClient)(nil)

// streamingClient implements acp.Client and translates ACP session updates
// into AG-UI SSE events written to the HTTP response.
type streamingClient struct {
	requestCtx context.Context
	w          io.Writer
	flusher    http.Flusher
	mu         sync.Mutex
	closed     bool
	runID      string
	messageID  string
	textOpen   bool
}

func newStreamingClient(ctx context.Context, w http.ResponseWriter, flusher http.Flusher, runID string) *streamingClient {
	return &streamingClient{
		requestCtx: ctx,
		w:          w,
		flusher:    flusher,
		runID:      runID,
	}
}

func (c *streamingClient) writeAndFlush(text string) {
	c.mu.Lock()
	defer c.mu.Unlock()

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

	defer func() {
		if recover() != nil {
			c.closed = true
		}
	}()

	if _, err := io.WriteString(c.w, text); err != nil {
		c.closed = true
		return
	}

	if c.flusher != nil {
		c.flusher.Flush()
	}
}

// writeSSEEvent encodes event as JSON and emits it as a single SSE frame.
func (c *streamingClient) writeSSEEvent(event map[string]any) {
	payload, err := json.Marshal(event)
	if err != nil {
		return
	}
	c.writeAndFlush("data: " + string(payload) + "\n\n")
}

// startRun emits RUN_STARTED and allocates a message id for the assistant
// text stream. RunID is preserved if the caller provided one via the AG-UI
// RunAgentInput payload.
func (c *streamingClient) startRun() {
	if c.runID == "" {
		c.runID = fmt.Sprintf("run-%d", time.Now().UnixNano())
	}
	if c.messageID == "" {
		c.messageID = fmt.Sprintf("msg-%d", time.Now().UnixNano())
	}
	c.writeSSEEvent(map[string]any{
		"type":  "RUN_STARTED",
		"runId": c.runID,
	})
}

// finishRun closes any open text message and emits RUN_FINISHED. The stop
// reason reported by the sidecar is passed through to AG-UI consumers.
func (c *streamingClient) finishRun(stopReason string) {
	if c.textOpen {
		c.writeSSEEvent(map[string]any{
			"type":      "TEXT_MESSAGE_END",
			"runId":     c.runID,
			"messageId": c.messageID,
		})
		c.textOpen = false
	}
	event := map[string]any{
		"type":  "RUN_FINISHED",
		"runId": c.runID,
	}
	if stopReason != "" {
		event["stopReason"] = stopReason
	}
	c.writeSSEEvent(event)
}

// failRun emits RUN_ERROR with the supplied message. It is safe to call from
// any handler error path; an open text message is closed first so frontends
// can finalize rendering before reporting the failure.
func (c *streamingClient) failRun(message string) {
	if c.textOpen {
		c.writeSSEEvent(map[string]any{
			"type":      "TEXT_MESSAGE_END",
			"runId":     c.runID,
			"messageId": c.messageID,
		})
		c.textOpen = false
	}
	c.writeSSEEvent(map[string]any{
		"type":    "RUN_ERROR",
		"runId":   c.runID,
		"message": message,
	})
}

// ensureTextStart emits TEXT_MESSAGE_START the first time streamed assistant
// text is observed. Subsequent chunks are emitted as TEXT_MESSAGE_CONTENT.
func (c *streamingClient) ensureTextStart() {
	if c.textOpen {
		return
	}
	c.writeSSEEvent(map[string]any{
		"type":      "TEXT_MESSAGE_START",
		"runId":     c.runID,
		"messageId": c.messageID,
	})
	c.textOpen = true
}

// RequestPermission always denies. The gateway advertises no filesystem or
// terminal capabilities, so any permission request is unexpected. Tool
// interactions (e.g. visualization) are handled via MCP tools passed by the
// frontend, not through ACP permissions.
func (*streamingClient) RequestPermission(context.Context, acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	return acp.RequestPermissionResponse{
		Outcome: acp.RequestPermissionOutcome{
			Cancelled: &acp.RequestPermissionOutcomeCancelled{},
		},
	}, nil
}

func (c *streamingClient) SessionUpdate(_ context.Context, n acp.SessionNotification) error {
	u := n.Update
	if u.AgentMessageChunk != nil {
		content := u.AgentMessageChunk.Content
		if content.Text != nil {
			c.ensureTextStart()
			c.writeSSEEvent(map[string]any{
				"type":      "TEXT_MESSAGE_CONTENT",
				"runId":     c.runID,
				"messageId": c.messageID,
				"delta":     content.Text.Text,
			})
		}
	}
	if u.ToolCall != nil {
		c.writeSSEEvent(map[string]any{
			"type":       "TOOL_CALL_START",
			"runId":      c.runID,
			"toolCallId": u.ToolCall.ToolCallId,
			"title":      u.ToolCall.Title,
			"kind":       u.ToolCall.Kind,
		})
		if u.ToolCall.RawInput != nil {
			c.writeSSEEvent(map[string]any{
				"type":       "TOOL_CALL_ARGS",
				"runId":      c.runID,
				"toolCallId": u.ToolCall.ToolCallId,
				"args":       u.ToolCall.RawInput,
			})
		}
	}
	if u.ToolCallUpdate != nil {
		if u.ToolCallUpdate.RawInput != nil {
			c.writeSSEEvent(map[string]any{
				"type":       "TOOL_CALL_ARGS",
				"runId":      c.runID,
				"toolCallId": u.ToolCallUpdate.ToolCallId,
				"args":       u.ToolCallUpdate.RawInput,
			})
		}
		if u.ToolCallUpdate.RawOutput != nil {
			c.writeSSEEvent(map[string]any{
				"type":       "TOOL_CALL_RESULT",
				"runId":      c.runID,
				"toolCallId": u.ToolCallUpdate.ToolCallId,
				"result":     u.ToolCallUpdate.RawOutput,
			})
		}
		status := valueOrUnknown(u.ToolCallUpdate.Status)
		if status == string(acp.ToolCallStatusCompleted) || status == string(acp.ToolCallStatusFailed) {
			c.writeSSEEvent(map[string]any{
				"type":       "TOOL_CALL_END",
				"runId":      c.runID,
				"toolCallId": u.ToolCallUpdate.ToolCallId,
				"status":     status,
			})
		}
	}
	return nil
}

func valueOrUnknown(v *acp.ToolCallStatus) string {
	if v == nil {
		return "unknown"
	}
	return string(*v)
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

func (*streamingClient) KillTerminalCommand(context.Context, acp.KillTerminalCommandRequest) (acp.KillTerminalCommandResponse, error) {
	return acp.KillTerminalCommandResponse{}, errNotSupported
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
