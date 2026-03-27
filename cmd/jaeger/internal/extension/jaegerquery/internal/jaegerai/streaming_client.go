// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/coder/acp-go-sdk"
)

// streamingClient implements acp.Client to handle callbacks and streaming text.
type streamingClient struct {
	requestCtx context.Context
	w          http.ResponseWriter
	flusher    http.Flusher
	mu         sync.Mutex
	closed     bool
	doneCh     chan struct{}
	doneOnce   sync.Once
	runID      string
	messageID  string
	textOpen   bool
}

func (c *streamingClient) signalDone() {
	c.doneOnce.Do(func() {
		if c.doneCh != nil {
			close(c.doneCh)
		}
	})
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
			c.signalDone()
			return
		default:
		}
	}

	defer func() {
		if recover() != nil {
			c.closed = true
			c.signalDone()
		}
	}()

	if _, err := io.WriteString(c.w, text); err != nil {
		c.closed = true
		c.signalDone()
		return
	}

	c.flusher.Flush()
}

func (c *streamingClient) writeSSEEvent(event map[string]any) {
	payload, err := json.Marshal(event)
	if err != nil {
		return
	}
	c.writeAndFlush("data: " + string(payload) + "\n\n")
}

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

func (c *streamingClient) finishRun() {
	if c.textOpen {
		c.writeSSEEvent(map[string]any{
			"type":      "TEXT_MESSAGE_END",
			"runId":     c.runID,
			"messageId": c.messageID,
		})
		c.textOpen = false
	}
	c.writeSSEEvent(map[string]any{
		"type":  "RUN_FINISHED",
		"runId": c.runID,
	})
}

func (c *streamingClient) failRun(message string) {
	c.writeSSEEvent(map[string]any{
		"type":    "RUN_ERROR",
		"runId":   c.runID,
		"message": message,
	})
}

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

func (c *streamingClient) waitForTurnCompletion(ctx context.Context, maxWait time.Duration) {
	if maxWait <= 0 {
		return
	}

	maxTimer := time.NewTimer(maxWait)
	defer maxTimer.Stop()

	select {
	case <-ctx.Done():
		return
	case <-maxTimer.C:
		return
	case <-c.doneCh:
		return
	}
}

func (*streamingClient) RequestPermission(_ context.Context, p acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	if len(p.Options) == 0 {
		return acp.RequestPermissionResponse{
			Outcome: acp.RequestPermissionOutcome{
				Cancelled: &acp.RequestPermissionOutcomeCancelled{},
			},
		}, nil
	}
	return acp.RequestPermissionResponse{
		Outcome: acp.RequestPermissionOutcome{
			Selected: &acp.RequestPermissionOutcomeSelected{OptionId: p.Options[0].OptionId},
		},
	}, nil
}

func (c *streamingClient) SessionUpdate(_ context.Context, n acp.SessionNotification) error {
	u := n.Update
	if u.AgentMessageChunk != nil {
		content := u.AgentMessageChunk.Content
		if content.Text != nil {
			if content.Text.Text == endOfTurnMarker {
				c.finishRun()
				c.signalDone()
			} else {
				c.ensureTextStart()
				c.writeSSEEvent(map[string]any{
					"type":      "TEXT_MESSAGE_CONTENT",
					"runId":     c.runID,
					"messageId": c.messageID,
					"delta":     content.Text.Text,
				})
			}
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

func (*streamingClient) WriteTextFile(_ context.Context, _ acp.WriteTextFileRequest) (acp.WriteTextFileResponse, error) {
	return acp.WriteTextFileResponse{}, nil
}

func (*streamingClient) ReadTextFile(_ context.Context, p acp.ReadTextFileRequest) (acp.ReadTextFileResponse, error) {
	return acp.ReadTextFileResponse{Content: "unsupported path: " + p.Path}, nil
}

func (*streamingClient) CreateTerminal(_ context.Context, _ acp.CreateTerminalRequest) (acp.CreateTerminalResponse, error) {
	return acp.CreateTerminalResponse{TerminalId: "t-1"}, nil
}

func (*streamingClient) KillTerminalCommand(_ context.Context, _ acp.KillTerminalCommandRequest) (acp.KillTerminalCommandResponse, error) {
	return acp.KillTerminalCommandResponse{}, nil
}

func (*streamingClient) ReleaseTerminal(_ context.Context, _ acp.ReleaseTerminalRequest) (acp.ReleaseTerminalResponse, error) {
	return acp.ReleaseTerminalResponse{}, nil
}

func (*streamingClient) TerminalOutput(_ context.Context, _ acp.TerminalOutputRequest) (acp.TerminalOutputResponse, error) {
	return acp.TerminalOutputResponse{Output: "ok", Truncated: false}, nil
}

func (*streamingClient) WaitForTerminalExit(_ context.Context, _ acp.WaitForTerminalExitRequest) (acp.WaitForTerminalExitResponse, error) {
	return acp.WaitForTerminalExitResponse{}, nil
}
