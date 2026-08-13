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

// WsReadWriteCloser adapts a WebSocket connection to io.ReadWriteCloser so the
// ACP SDK can drive it. A WebSocket delivers discrete messages, each ending in
// io.EOF, while the SDK reads one continuous stream and splits it on newlines.
// Converting between the two is sound because ACP messages "are delimited by
// newlines (\n), and MUST NOT contain embedded newlines":
// https://agentclientprotocol.com/protocol/v1/transports
//
// A single goroutine must own Read, which keeps unsynchronized framing state.
type WsReadWriteCloser struct {
	conn   *websocket.Conn
	r      io.Reader
	logger *zap.Logger
	// One message can span several Read calls, because gorilla clamps each Read
	// to the current WebSocket frame. The delimiter therefore goes in when the
	// message ends rather than when a Read ends, and w.r == nil marks that gap.
	messageUnterminated bool
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
		if w.r == nil {
			// If the last seen message lacked a delimiter, return the newline
			// before opening the next reader.
			if w.messageUnterminated {
				w.messageUnterminated = false
				p[0] = '\n'
				return 1, nil
			}
			_, r, err := w.conn.NextReader()
			if err != nil {
				return 0, err
			}
			w.r = r
		}

		n, err := w.r.Read(p)
		if n > 0 {
			// Tracked here instead of the EOF case because websocket may return
			// the payload and EOF via two consecutive calls.
			w.messageUnterminated = !bytes.HasSuffix(p[:n], []byte("\n"))
		}
		if err == io.EOF {
			w.r = nil
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
