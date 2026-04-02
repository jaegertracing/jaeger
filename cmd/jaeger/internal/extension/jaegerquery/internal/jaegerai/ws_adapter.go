// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"fmt"
	"io"

	"github.com/gorilla/websocket"
)

// WsReadWriteCloser wraps a gorilla websocket to implement io.ReadWriteCloser.
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
