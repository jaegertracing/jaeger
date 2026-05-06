// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// WsReadWriteCloser wraps a gorilla websocket to implement io.ReadWriteCloser.
type WsReadWriteCloser struct {
	conn   *websocket.Conn
	r      io.Reader
	logger *zap.Logger
}

// NewWsAdapter wraps an existing websocket connection.
func NewWsAdapter(conn *websocket.Conn, logger *zap.Logger) *WsReadWriteCloser {
	return &WsReadWriteCloser{conn: conn, logger: logger}
}

// DialWsAdapter dials a websocket endpoint and returns the adapter.
// The caller must call Close() to release the connection.
// On error, gorilla closes resp.Body internally (wraps it in io.NopCloser),
// so we only read it here for diagnostic logging.
func DialWsAdapter(ctx context.Context, url string, requestHeader http.Header, logger *zap.Logger) (*WsReadWriteCloser, error) {
	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	conn, resp, err := dialer.DialContext(ctx, url, requestHeader) //nolint:bodyclose // gorilla wraps resp.Body in io.NopCloser; no close needed
	if err != nil {
		if resp != nil {
			body, _ := io.ReadAll(resp.Body)
			logger.Error("WebSocket dial failed",
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
	for {
		if w.r == nil {
			_, r, err := w.conn.NextReader()
			if err != nil {
				return 0, err
			}
			w.r = r
		}

		n, err := w.r.Read(p)
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
