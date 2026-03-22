// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func TestWsReadWriteCloserRoundTrip(t *testing.T) {
	t.Parallel()

	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		msgType, payload, err := conn.ReadMessage()
		if err != nil {
			return
		}
		_ = conn.WriteMessage(msgType, payload)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.DialContext(context.Background(), wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	adapter := NewWsAdapter(conn)
	original := []byte("hello from ws adapter")

	n, err := adapter.Write(original)
	if err != nil {
		t.Fatalf("adapter write failed: %v", err)
	}
	if n != len(original) {
		t.Fatalf("unexpected write count: got %d want %d", n, len(original))
	}

	buf := make([]byte, len(original))
	if _, err := io.ReadFull(adapter, buf); err != nil {
		t.Fatalf("adapter read failed: %v", err)
	}

	if !bytes.Equal(buf, original) {
		t.Fatalf("round-trip mismatch: got %q want %q", string(buf), string(original))
	}
}
