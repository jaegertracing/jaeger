// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"
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
				c.signalDone()
			} else {
				c.writeAndFlush(content.Text.Text)
			}
		}
	}
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
