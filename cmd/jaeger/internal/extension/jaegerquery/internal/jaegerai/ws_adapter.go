// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// newline is the JSON-RPC line delimiter the reader above this adapter expects.
var newline = []byte{'\n'}

// WsReadWriteCloser wraps a gorilla websocket to implement io.ReadWriteCloser.
type WsReadWriteCloser struct {
	conn   *websocket.Conn
	r      io.Reader
	logger *zap.Logger
	// ACP frames exactly one JSON-RPC object per WebSocket message, but the
	// codec reading from us is line-oriented, so a message that arrives without
	// a trailing newline would leave it waiting for a delimiter that never
	// comes. Agents built on the ACP SDK's own WebSocket server (acp.ws) send
	// exactly that shape, so the delimiter is re-inserted at the message
	// boundary. These three fields track where that boundary is.
	pendingNewline   bool
	sawData          bool
	endedWithNewline bool
}

// NewWsAdapter wraps an existing websocket connection.
func NewWsAdapter(conn *websocket.Conn, logger *zap.Logger) *WsReadWriteCloser {
	return &WsReadWriteCloser{conn: conn, logger: logger}
}

// DialWsAdapter dials a websocket endpoint and returns the adapter.
// The caller must call Close() to release the connection.
// On error, gorilla closes resp.Body internally (wraps it in io.NopCloser),
// so we only read it here for diagnostic logging.
func DialWsAdapter(ctx context.Context, url string, logger *zap.Logger) (*WsReadWriteCloser, error) {
	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	conn, resp, err := dialer.DialContext(ctx, url, nil) //nolint:bodyclose // gorilla wraps resp.Body in io.NopCloser; no close needed
	if err != nil {
		if resp != nil {
			body, _ := io.ReadAll(resp.Body)
			logger.Error(
				"WebSocket dial failed",
				zap.Int("status", resp.StatusCode),
				zap.String("body", string(body)),
				zap.Error(err),
			)
		}
		return nil, fmt.Errorf("websocket dial %s: %w", url, err)
	}
	return &WsReadWriteCloser{conn: conn, logger: logger}, nil
}

func (w *WsReadWriteCloser) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	for {
		// A message boundary was crossed without a newline; supply it before
		// reading anything further, so the reader above sees one framed line.
		if w.pendingNewline {
			w.pendingNewline = false
			p[0] = '\n'
			return 1, nil
		}

		if w.r == nil {
			_, r, err := w.conn.NextReader()
			if err != nil {
				return 0, err
			}
			w.r = r
			w.sawData = false
			w.endedWithNewline = false
		}

		n, err := w.r.Read(p)
		if n > 0 {
			w.sawData = true
			w.endedWithNewline = bytes.HasSuffix(p[:n], newline)
		}
		if err == io.EOF {
			w.r = nil
			// Empty messages carry no frame, so they need no delimiter.
			if w.sawData && !w.endedWithNewline {
				w.pendingNewline = true
			}
			if n > 0 {
				return n, nil
			}
			continue
		}
		return n, err
	}
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
