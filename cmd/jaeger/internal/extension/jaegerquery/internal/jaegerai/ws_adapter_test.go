// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
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
	require.NoError(t, err, "dial websocket")
	defer conn.Close()

	adapter := NewWsAdapter(conn, zap.NewNop())
	original := []byte("hello from ws adapter")

	n, err := adapter.Write(original)
	require.NoError(t, err, "adapter write failed")
	require.Equal(t, len(original), n, "unexpected write count")

	buf := make([]byte, len(original))
	_, err = io.ReadFull(adapter, buf)
	require.NoError(t, err, "adapter read failed")

	assert.Equal(t, original, buf, "round-trip mismatch: got %q want %q", string(buf), string(original))
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
	require.NoError(t, err, "dial websocket")

	adapter := NewWsAdapter(conn, zap.NewNop())
	require.NoError(t, adapter.Close(), "close failed")
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
	require.NoError(t, err, "dial websocket")
	defer conn.Close()

	adapter := NewWsAdapter(conn, zap.NewNop())
	buf := make([]byte, 16)
	_, err = adapter.Read(buf)
	require.Error(t, err, "expected read error after peer close")
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
	require.NoError(t, err, "dial websocket")
	_ = conn.Close()

	adapter := NewWsAdapter(conn, zap.NewNop())
	_, err = adapter.Write([]byte("should fail"))
	require.Error(t, err, "expected write error")
}

func TestDialWsAdapterSuccess(t *testing.T) {
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
	adapter, err := DialWsAdapter(context.Background(), wsURL, nil, zap.NewNop())
	require.NoError(t, err, "DialWsAdapter should succeed")
	require.NoError(t, adapter.Close(), "close should succeed")
}

func TestDialWsAdapterFailure(t *testing.T) {
	t.Parallel()

	_, err := DialWsAdapter(context.Background(), "ws://127.0.0.1:1", nil, zap.NewNop())
	require.Error(t, err, "DialWsAdapter should fail for unreachable host")
}

func TestDialWsAdapterHTTPErrorLogsResponse(t *testing.T) {
	t.Parallel()

	// Server returns a plain HTTP 403 instead of upgrading to WebSocket.
	// DialWsAdapter should log the status and body.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	_, err := DialWsAdapter(context.Background(), wsURL, nil, zap.NewNop())
	require.Error(t, err, "DialWsAdapter should fail when server rejects upgrade")
	require.Contains(t, err.Error(), "websocket dial")
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
	require.NoError(t, err, "dial websocket")
	defer conn.Close()

	adapter := NewWsAdapter(conn, zap.NewNop())
	buf := make([]byte, 16)

	n, err := adapter.Read(buf)
	require.NoError(t, err, "unexpected read error")
	require.Equal(t, 2, n, "unexpected read size")
	assert.Equal(t, "hi", string(buf[:n]), "unexpected read payload")
}

func TestWsAdapterReadSmallBufferReturnsPartialBytes(t *testing.T) {
	t.Parallel()

	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		// Send a message longer than the read buffer.
		_ = conn.WriteMessage(websocket.TextMessage, []byte("abcdef"))
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.DialContext(context.Background(), wsURL, nil)
	require.NoError(t, err, "dial websocket")
	defer conn.Close()

	adapter := NewWsAdapter(conn, zap.NewNop())

	// Use a 4-byte buffer for a 6-byte message — first read returns 4 bytes,
	// second read returns the remaining 2 bytes alongside the internal EOF,
	// which exercises the "n > 0 on EOF" branch.
	buf := make([]byte, 4)
	n, err := adapter.Read(buf)
	require.NoError(t, err)
	require.Equal(t, 4, n)
	assert.Equal(t, "abcd", string(buf[:n]))

	n, err = adapter.Read(buf)
	require.NoError(t, err)
	require.Equal(t, 2, n)
	assert.Equal(t, "ef", string(buf[:n]))
}

func TestWsAdapterReadMultipleMessagesSmallBuffer(t *testing.T) {
	t.Parallel()

	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		// Send two separate messages — the reader must transition between them
		// via the EOF→continue loop in Read.
		_ = conn.WriteMessage(websocket.TextMessage, []byte("aa"))
		_ = conn.WriteMessage(websocket.TextMessage, []byte("bb"))
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.DialContext(context.Background(), wsURL, nil)
	require.NoError(t, err)
	defer conn.Close()

	adapter := NewWsAdapter(conn, zap.NewNop())
	// Read exactly 2 bytes — matches first message, triggers EOF with n=0 internally.
	buf := make([]byte, 2)
	n, err := adapter.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, "aa", string(buf[:n]))

	// Second read gets the next message after the internal EOF→continue.
	n, err = adapter.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, "bb", string(buf[:n]))
}

// bytesEOFReader returns all data and io.EOF in a single Read call,
// exercising the "n > 0 on EOF" branch in WsReadWriteCloser.Read.
type bytesEOFReader struct{ data []byte }

func (r *bytesEOFReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	if len(r.data) == 0 {
		return n, io.EOF
	}
	return n, nil
}

func TestWsAdapterReadReturnsPartialBytesOnEOF(t *testing.T) {
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
	require.NoError(t, err)
	defer conn.Close()

	adapter := NewWsAdapter(conn, zap.NewNop())
	// Inject a reader that returns n > 0 and io.EOF simultaneously.
	adapter.r = &bytesEOFReader{data: []byte("hello")}

	buf := make([]byte, 16)
	n, err := adapter.Read(buf)
	require.NoError(t, err, "should swallow EOF when bytes are returned")
	assert.Equal(t, "hello", string(buf[:n]))
}
