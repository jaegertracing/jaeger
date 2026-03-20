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
	"github.com/gorilla/websocket"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
)

const endOfTurnMarker = "__END_OF_TURN__"

// WsReadWriteCloser wraps a gorilla websocket to implement io.ReadWriteCloser
type WsReadWriteCloser struct {
	conn *websocket.Conn
	r    io.Reader
}

func NewWsAdapter(conn *websocket.Conn) *WsReadWriteCloser {
	return &WsReadWriteCloser{conn: conn}
}

func (w *WsReadWriteCloser) Read(p []byte) (int, error) {
	if w.r == nil {
		messageType, r, err := w.conn.NextReader()
		if err != nil {
			return 0, err
		}
		if messageType != websocket.TextMessage && messageType != websocket.BinaryMessage {
			return 0, fmt.Errorf("unexpected message type: %d", messageType)
		}
		w.r = r
	}

	n, err := w.r.Read(p)
	if err == io.EOF {
		w.r = nil
		if n > 0 {
			return n, nil
		}
		return w.Read(p)
	}
	return n, err
}

func (w *WsReadWriteCloser) Write(p []byte) (int, error) {
	err := w.conn.WriteMessage(websocket.TextMessage, p)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func (w *WsReadWriteCloser) Close() error {
	return w.conn.Close()
}

// ChatRequest is the incoming payload
type ChatRequest struct {
	Prompt string `json:"prompt"`
}

// ChatHandler manages the AI gateway requests
type ChatHandler struct {
	Logger       *zap.Logger
	QueryService *querysvc.QueryService
}

func NewChatHandler(logger *zap.Logger, queryService *querysvc.QueryService) *ChatHandler {
	return &ChatHandler{Logger: logger, QueryService: queryService}
}

// streamingClient implements acp.Client to handle callbacks and streaming text
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

func (h *ChatHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ctx := r.Context()
	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	conn, resp, err := dialer.DialContext(ctx, "ws://localhost:9000", nil)
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		h.Logger.Error("Failed to dial ACP sidecar", zap.Error(err))
		http.Error(w, "Failed to connect to agent backend", http.StatusBadGateway)
		return
	}
	defer conn.Close()

	adapter := NewWsAdapter(conn)

	clientImpl := &streamingClient{
		requestCtx: ctx,
		w:          w,
		flusher:    flusher,
		doneCh:     make(chan struct{}),
	}
	acpConn := acp.NewClientSideConnection(clientImpl, adapter, adapter)

	acpCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	_, err = acpConn.Initialize(acpCtx, acp.InitializeRequest{
		ProtocolVersion: acp.ProtocolVersionNumber,
		ClientCapabilities: acp.ClientCapabilities{
			Fs:       acp.FileSystemCapability{ReadTextFile: false, WriteTextFile: false},
			Terminal: false,
		},
		ClientInfo: &acp.Implementation{
			Name:    "jaeger-ai-gateway",
			Version: "0.1.0",
		},
	})
	if err != nil {
		fmt.Fprintf(w, "Error initializing agent: %v\n", err)
		return
	}

	sess, err := acpConn.NewSession(acpCtx, acp.NewSessionRequest{
		Cwd:        "/",
		McpServers: []acp.McpServer{},
	})
	if err != nil {
		fmt.Fprintf(w, "Error creating session: %v\n", err)
		return
	}

	// This is blocking until the agent finishes processing the prompt
	_, err = acpConn.Prompt(acpCtx, acp.PromptRequest{
		SessionId: sess.SessionId,
		Prompt:    []acp.ContentBlock{acp.TextBlock(req.Prompt)},
	})
	if err != nil {
		fmt.Fprintf(w, "Error starting prompt: %v\n", err)
		return
	}

	// Wait for explicit end-of-turn marker from the sidecar, with timeout fallback.
	clientImpl.waitForTurnCompletion(acpCtx, 2*time.Second)
}
