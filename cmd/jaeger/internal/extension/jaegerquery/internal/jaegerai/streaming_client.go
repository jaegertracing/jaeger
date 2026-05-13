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
	"go.uber.org/zap"
)

var _ acp.Client = (*streamingClient)(nil)

// streamingClient implements acp.Client to handle callbacks and streaming text.
type streamingClient struct {
	requestCtx context.Context
	w          io.Writer
	mu         sync.Mutex
	closed     bool
}

func newStreamingClient(ctx context.Context, w http.ResponseWriter) *streamingClient {
	return &streamingClient{
		requestCtx: ctx,
		w:          w,
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

	if flusher, ok := c.w.(http.Flusher); ok {
		flusher.Flush()
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
	// TODO: upgrade to AG-UI https://docs.ag-ui.com/concepts/events
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
