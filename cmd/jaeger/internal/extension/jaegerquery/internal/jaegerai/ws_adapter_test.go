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
	"time"

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

func TestWsAdapterClose(t *testing.T) {
	t.Parallel()

	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		_, _, _ = conn.ReadMessage()
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.DialContext(context.Background(), wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}

	adapter := NewWsAdapter(conn)
	if err := adapter.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}
}

func TestWsAdapterReadAfterPeerClose(t *testing.T) {
	t.Parallel()

	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		_ = conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "bye"),
			time.Now().Add(time.Second),
		)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.DialContext(context.Background(), wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	adapter := NewWsAdapter(conn)
	buf := make([]byte, 16)
	_, err = adapter.Read(buf)
	if err == nil {
		t.Fatal("expected read error after peer close")
	}
}

func TestWsAdapterWriteError(t *testing.T) {
	t.Parallel()

	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_, _, _ = conn.ReadMessage()
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.DialContext(context.Background(), wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	_ = conn.Close()

	adapter := NewWsAdapter(conn)
	_, err = adapter.Write([]byte("should fail"))
	if err == nil {
		t.Fatal("expected write error")
	}
}

func TestWsAdapterReadReturnsBytesOnEOF(t *testing.T) {
	t.Parallel()

	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		_ = conn.WriteMessage(websocket.TextMessage, []byte("hi"))
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.DialContext(context.Background(), wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	adapter := NewWsAdapter(conn)
	buf := make([]byte, 16)

	n, err := adapter.Read(buf)
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}
	if n != 2 {
		t.Fatalf("unexpected read size: got %d want %d", n, 2)
	}
	if got := string(buf[:n]); got != "hi" {
		t.Fatalf("unexpected read payload: got %q want %q", got, "hi")
	}
}
