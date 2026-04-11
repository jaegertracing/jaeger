// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/coder/acp-go-sdk"
)

var _ acp.Client = (*streamingClient)(nil)

// streamingClient implements acp.Client to handle callbacks and streaming text.
type streamingClient struct {
	requestCtx context.Context
	w          http.ResponseWriter
	flusher    http.Flusher
	mu         sync.Mutex
	closed     bool
	doneCh     chan struct{}
	doneOnce   sync.Once
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

// waitForTurnCompletion is a short grace period after Prompt() returns,
// allowing in-flight SessionUpdate callbacks to finish flushing to the HTTP
// response. acp-go-sdk dispatches these callbacks in goroutines, so Prompt()
// may return before the last streamed chunk is written. On success this acts
// as a brief sleep (maxWait); on error or context cancellation doneCh fires
// and the wait exits early.
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
			c.writeAndFlush(content.Text.Text)
		}
	}
	// [tool_call] and [tool_result] are informational markers streamed to the
	// HTTP response for the UI to display progress. They are not part of the
	// ACP protocol — just human-readable status lines in the text stream.
	if u.ToolCall != nil {
		c.writeAndFlush(fmt.Sprintf("\n[tool_call] %s\n", u.ToolCall.Title))
	}
	if u.ToolCallUpdate != nil {
		c.writeAndFlush(fmt.Sprintf("\n[tool_result] id=%s status=%s\n", u.ToolCallUpdate.ToolCallId, valueOrUnknown(u.ToolCallUpdate.Status)))
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
