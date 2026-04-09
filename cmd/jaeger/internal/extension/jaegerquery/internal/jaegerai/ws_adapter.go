// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"
	"io"
	"time"

	"github.com/gorilla/websocket"
)

// WsReadWriteCloser wraps a gorilla websocket to implement io.ReadWriteCloser.
type WsReadWriteCloser struct {
	conn     *websocket.Conn
	r        io.Reader
	respBody io.Closer // HTTP response body from the dial handshake, if any
}

// NewWsAdapter wraps an existing websocket connection.
func NewWsAdapter(conn *websocket.Conn) *WsReadWriteCloser {
	return &WsReadWriteCloser{conn: conn}
}

// DialWsAdapter dials a websocket endpoint and returns the adapter.
// The caller must call Close() to release the connection and any dial response resources.
func DialWsAdapter(ctx context.Context, url string) (*WsReadWriteCloser, error) {
	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	conn, resp, err := dialer.DialContext(ctx, url, nil)
	var respBody io.Closer
	if resp != nil && resp.Body != nil {
		respBody = resp.Body
	}
	if err != nil {
		if respBody != nil {
			respBody.Close()
		}
		return nil, err
	}
	return &WsReadWriteCloser{conn: conn, respBody: respBody}, nil
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
	connErr := w.conn.Close()
	if w.respBody != nil {
		if err := w.respBody.Close(); err != nil && connErr == nil {
			connErr = err
		}
	}
	return connErr
}
