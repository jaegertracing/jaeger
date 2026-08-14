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
	adapter, err := DialWsAdapter(context.Background(), wsURL, zap.NewNop())
	require.NoError(t, err, "DialWsAdapter should succeed")
	require.NoError(t, adapter.Close(), "close should succeed")
}

func TestDialWsAdapterFailure(t *testing.T) {
	t.Parallel()

	_, err := DialWsAdapter(context.Background(), "ws://127.0.0.1:1", zap.NewNop())
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
	_, err := DialWsAdapter(context.Background(), wsURL, zap.NewNop())
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

	// Each message ends at a line boundary, so the delimiter the sender omitted is
	// supplied here before the next message is read.
	n, err = adapter.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, "\n", string(buf[:n]))

	// Second message follows, after the internal EOF→continue.
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

// serveMessages upgrades one connection and writes each payload as its own
// WebSocket text message, mirroring how an ACP agent frames JSON-RPC.
func serveMessages(t *testing.T, payloads ...string) *websocket.Conn {
	t.Helper()

	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for _, payload := range payloads {
			if err := conn.WriteMessage(websocket.TextMessage, []byte(payload)); err != nil {
				return
			}
		}
		// Hold the connection open so the client controls teardown.
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	t.Cleanup(server.Close)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.DialContext(context.Background(), wsURL, nil)
	require.NoError(t, err, "dial websocket")
	// Without a deadline, an adapter that stops emitting delimiters would leave
	// readFramed blocked until the whole test binary times out.
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(10*time.Second)))
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// readFramed reads through bufSize-sized buffers until it has as many bytes as
// want, so each caller states the framing it expects instead of a byte count.
func readFramed(t *testing.T, adapter *WsReadWriteCloser, want string, bufSize int) string {
	t.Helper()

	buf := make([]byte, bufSize)
	var got strings.Builder
	for got.Len() < len(want) {
		n, err := adapter.Read(buf)
		require.NoError(t, err)
		got.Write(buf[:n])
	}
	return got.String()
}

// Agents that frame one JSON-RPC object per WebSocket message send no trailing
// newline, which stalls the bufio.Scanner the ACP SDK reads us with, so the
// adapter supplies the delimiter itself.
func TestWsReadWriteCloserFramesMessagesAsLines(t *testing.T) {
	t.Parallel()

	first := `{"jsonrpc":"2.0","id":1,"result":{}}`
	second := `{"jsonrpc":"2.0","id":2,"result":{}}`
	conn := serveMessages(t, first, second)
	adapter := NewWsAdapter(conn, zap.NewNop())

	got := readFramed(t, adapter, first+"\n"+second+"\n", 256)

	// The property under test is the framing: each WebSocket message must surface
	// as exactly one newline-terminated line, each still parseable on its own.
	require.True(t, strings.HasSuffix(got, "\n"), "stream must end at a line boundary")
	lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
	require.Len(t, lines, 2, "two messages should produce two lines")
	assert.JSONEq(t, first, lines[0])
	assert.JSONEq(t, second, lines[1])
}

// A newline the agent already supplied must not be doubled, or the scanner sees
// a stray blank line between messages.
func TestWsReadWriteCloserDoesNotDoubleExistingNewline(t *testing.T) {
	t.Parallel()

	conn := serveMessages(t, "{\"id\":1}\n", "{\"id\":2}\n")
	adapter := NewWsAdapter(conn, zap.NewNop())

	want := "{\"id\":1}\n{\"id\":2}\n"
	assert.Equal(t, want, readFramed(t, adapter, want, 256))
}

// The injected newline must survive a buffer too small to hold the message and
// its delimiter in one Read.
func TestWsReadWriteCloserSmallBuffer(t *testing.T) {
	t.Parallel()

	conn := serveMessages(t, `{"id":1}`)
	adapter := NewWsAdapter(conn, zap.NewNop())

	want := "{\"id\":1}\n"
	assert.Equal(t, want, readFramed(t, adapter, want, 3))
}

// An empty message carries no JSON-RPC object, so it must not produce a stray
// blank line ahead of the next real one.
func TestWsReadWriteCloserSkipsEmptyMessages(t *testing.T) {
	t.Parallel()

	conn := serveMessages(t, "", `{"id":1}`)
	adapter := NewWsAdapter(conn, zap.NewNop())

	want := "{\"id\":1}\n"
	assert.Equal(t, want, readFramed(t, adapter, want, 256))
}

// A zero-length buffer must not consume a message or panic indexing p[0].
func TestWsReadWriteCloserZeroLengthBuffer(t *testing.T) {
	t.Parallel()

	conn := serveMessages(t, `{"id":1}`)
	adapter := NewWsAdapter(conn, zap.NewNop())

	n, err := adapter.Read(nil)
	require.NoError(t, err)
	assert.Zero(t, n)

	// The message was left pending, so a real buffer still frames it.
	want := "{\"id\":1}\n"
	assert.Equal(t, want, readFramed(t, adapter, want, 256))
}

// A WebSocket message arrives complete, but gorilla still delivers it across
// several Read calls when the sender fragments it into continuation frames,
// because each Read is clamped to the current frame. The delimiter therefore
// belongs at the end of the message, not at the end of a Read.
func TestWsReadWriteCloserFragmentedMessageIsOneLine(t *testing.T) {
	t.Parallel()

	payload := `{"jsonrpc":"2.0","id":1,"result":{"pad":"` + strings.Repeat("x", 300) + `"}}`
	upgrader := websocket.Upgrader{
		CheckOrigin:     func(*http.Request) bool { return true },
		WriteBufferSize: 16, // far below the payload, so gorilla fragments it
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		writer, err := conn.NextWriter(websocket.TextMessage)
		if err != nil {
			return
		}
		if _, err := writer.Write([]byte(payload)); err != nil {
			return
		}
		_ = writer.Close()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	t.Cleanup(server.Close)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.DialContext(context.Background(), wsURL, nil)
	require.NoError(t, err, "dial websocket")
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(10*time.Second)))
	t.Cleanup(func() { _ = conn.Close() })
	adapter := NewWsAdapter(conn, zap.NewNop())

	// The buffer is larger than the whole message, so a single Read would suffice
	// if fragments did not exist.
	buf := make([]byte, 4096)
	var got strings.Builder
	reads := 0
	for got.Len() < len(payload)+1 {
		n, err := adapter.Read(buf)
		require.NoError(t, err)
		got.Write(buf[:n])
		reads++
	}

	assert.Greater(t, reads, 1, "a fragmented message must span multiple Read calls")
	assert.Equal(t, payload+"\n", got.String())
}
