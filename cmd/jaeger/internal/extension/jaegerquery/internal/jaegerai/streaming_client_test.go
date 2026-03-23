// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/acp-go-sdk"
)

type countingFlusher struct {
	count int
}

func (f *countingFlusher) Flush() {
	f.count++
}

type errResponseWriter struct {
	header http.Header
}

func (w *errResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (*errResponseWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

func (*errResponseWriter) WriteHeader(int) {}

type panicResponseWriter struct {
	header http.Header
}

func (w *panicResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (*panicResponseWriter) Write([]byte) (int, error) {
	panic("boom")
}

func (*panicResponseWriter) WriteHeader(int) {}

func TestStreamingClientWriteAndFlushWritesText(t *testing.T) {
	rec := httptest.NewRecorder()
	flusher := &countingFlusher{}
	c := &streamingClient{
		requestCtx: context.Background(),
		w:          rec,
		flusher:    flusher,
		doneCh:     make(chan struct{}),
	}

	c.writeAndFlush("hello")

	if got, want := rec.Body.String(), "hello"; got != want {
		t.Fatalf("unexpected body: got %q want %q", got, want)
	}
	if flusher.count != 1 {
		t.Fatalf("expected one flush, got %d", flusher.count)
	}
}

func TestStreamingClientWriteAndFlushContextDoneSignalsDone(t *testing.T) {
	rec := httptest.NewRecorder()
	flusher := &countingFlusher{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	c := &streamingClient{
		requestCtx: ctx,
		w:          rec,
		flusher:    flusher,
		doneCh:     make(chan struct{}),
	}

	c.writeAndFlush("ignored")

	if !c.closed {
		t.Fatal("expected client to be closed")
	}
	select {
	case <-c.doneCh:
	default:
		t.Fatal("expected done channel to be closed")
	}
	if got := rec.Body.String(); got != "" {
		t.Fatalf("expected empty body, got %q", got)
	}
}

func TestStreamingClientWriteAndFlushWriteErrorSignalsDone(t *testing.T) {
	c := &streamingClient{
		requestCtx: context.Background(),
		w:          &errResponseWriter{},
		flusher:    &countingFlusher{},
		doneCh:     make(chan struct{}),
	}

	c.writeAndFlush("hello")

	if !c.closed {
		t.Fatal("expected client to be closed on write error")
	}
	select {
	case <-c.doneCh:
	default:
		t.Fatal("expected done channel to be closed")
	}
}

func TestStreamingClientWriteAndFlushRecoverSignalsDone(t *testing.T) {
	c := &streamingClient{
		requestCtx: context.Background(),
		w:          &panicResponseWriter{},
		flusher:    &countingFlusher{},
		doneCh:     make(chan struct{}),
	}

	c.writeAndFlush("hello")

	if !c.closed {
		t.Fatal("expected client to be closed after panic")
	}
	select {
	case <-c.doneCh:
	default:
		t.Fatal("expected done channel to be closed")
	}
}

func TestStreamingClientWriteAndFlushNoopWhenClosed(t *testing.T) {
	rec := httptest.NewRecorder()
	flusher := &countingFlusher{}
	c := &streamingClient{
		requestCtx: context.Background(),
		w:          rec,
		flusher:    flusher,
		closed:     true,
		doneCh:     make(chan struct{}),
	}

	c.writeAndFlush("ignored")

	if got := rec.Body.String(); got != "" {
		t.Fatalf("expected no writes when already closed, got %q", got)
	}
	if flusher.count != 0 {
		t.Fatalf("expected no flush when already closed, got %d", flusher.count)
	}
}

func TestStreamingClientWaitForTurnCompletion(t *testing.T) {
	t.Run("returns when done signaled", func(t *testing.T) {
		c := &streamingClient{doneCh: make(chan struct{})}
		close(c.doneCh)
		start := time.Now()
		c.waitForTurnCompletion(context.Background(), time.Second)
		if time.Since(start) > 200*time.Millisecond {
			t.Fatal("wait should return quickly when done channel is closed")
		}
	})

	t.Run("returns on context cancel", func(t *testing.T) {
		c := &streamingClient{doneCh: make(chan struct{})}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		start := time.Now()
		c.waitForTurnCompletion(ctx, time.Second)
		if time.Since(start) > 200*time.Millisecond {
			t.Fatal("wait should return quickly when context is canceled")
		}
	})

	t.Run("returns on timeout", func(t *testing.T) {
		start := time.Now()
		c2 := &streamingClient{doneCh: make(chan struct{})}
		c2.waitForTurnCompletion(context.Background(), 20*time.Millisecond)
		if time.Since(start) < 20*time.Millisecond {
			t.Fatal("wait returned before timeout elapsed")
		}
	})

	t.Run("returns immediately for non-positive timeout", func(t *testing.T) {
		start := time.Now()
		c2 := &streamingClient{doneCh: make(chan struct{})}
		c2.waitForTurnCompletion(context.Background(), 0)
		if time.Since(start) > 200*time.Millisecond {
			t.Fatal("wait should return immediately for non-positive timeout")
		}
	})
}

func TestStreamingClientRequestPermission(t *testing.T) {
	c := &streamingClient{}

	resp, err := c.RequestPermission(context.Background(), acp.RequestPermissionRequest{})
	if err != nil {
		t.Fatalf("request permission unexpected error: %v", err)
	}
	if resp.Outcome.Cancelled == nil {
		t.Fatal("expected cancelled outcome when options are empty")
	}

	resp, err = c.RequestPermission(context.Background(), acp.RequestPermissionRequest{
		Options: []acp.PermissionOption{{
			OptionId: "opt-1",
			Name:     "allow",
			Kind:     acp.PermissionOptionKindAllowOnce,
		}},
	})
	if err != nil {
		t.Fatalf("request permission unexpected error: %v", err)
	}
	if resp.Outcome.Selected == nil || resp.Outcome.Selected.OptionId != "opt-1" {
		t.Fatalf("expected selected option opt-1, got %+v", resp.Outcome)
	}
}

func TestStreamingClientSessionUpdate(t *testing.T) {
	rec := httptest.NewRecorder()
	flusher := &countingFlusher{}
	c := &streamingClient{
		requestCtx: context.Background(),
		w:          rec,
		flusher:    flusher,
		doneCh:     make(chan struct{}),
	}

	status := acp.ToolCallStatusCompleted
	err := c.SessionUpdate(context.Background(), acp.SessionNotification{
		Update: acp.SessionUpdate{
			AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{Content: acp.TextBlock("chunk")},
			ToolCall:          &acp.SessionUpdateToolCall{Title: "search traces"},
			ToolCallUpdate:    &acp.SessionToolCallUpdate{ToolCallId: "tool-1", Status: &status},
		},
	})
	if err != nil {
		t.Fatalf("session update returned error: %v", err)
	}

	got := rec.Body.String()
	if !strings.Contains(got, "chunk") {
		t.Fatalf("expected chunk output, got %q", got)
	}
	if !strings.Contains(got, "[tool_call] search traces") {
		t.Fatalf("expected tool call output, got %q", got)
	}
	if !strings.Contains(got, "[tool_result] id=tool-1 status=completed") {
		t.Fatalf("expected tool result output, got %q", got)
	}

	err = c.SessionUpdate(context.Background(), acp.SessionNotification{
		Update: acp.SessionUpdate{
			AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{Content: acp.TextBlock(endOfTurnMarker)},
		},
	})
	if err != nil {
		t.Fatalf("session update returned error: %v", err)
	}
	select {
	case <-c.doneCh:
	default:
		t.Fatal("expected done channel to be closed when marker is received")
	}
}

func TestStreamingClientUtilityMethods(t *testing.T) {
	c := &streamingClient{}

	if got := valueOrUnknown(nil); got != "unknown" {
		t.Fatalf("valueOrUnknown nil mismatch: got %q", got)
	}
	status := acp.ToolCallStatusInProgress
	if got := valueOrUnknown(&status); got != "in_progress" {
		t.Fatalf("valueOrUnknown status mismatch: got %q", got)
	}

	if _, err := c.WriteTextFile(context.Background(), acp.WriteTextFileRequest{}); err != nil {
		t.Fatalf("write text file error: %v", err)
	}
	readResp, err := c.ReadTextFile(context.Background(), acp.ReadTextFileRequest{Path: "/tmp/nope"})
	if err != nil {
		t.Fatalf("read text file error: %v", err)
	}
	if !strings.Contains(readResp.Content, "unsupported path: /tmp/nope") {
		t.Fatalf("unexpected read text file response: %q", readResp.Content)
	}

	createResp, err := c.CreateTerminal(context.Background(), acp.CreateTerminalRequest{})
	if err != nil {
		t.Fatalf("create terminal error: %v", err)
	}
	if createResp.TerminalId != "t-1" {
		t.Fatalf("unexpected terminal id: %q", createResp.TerminalId)
	}

	if _, err := c.KillTerminalCommand(context.Background(), acp.KillTerminalCommandRequest{}); err != nil {
		t.Fatalf("kill terminal command error: %v", err)
	}
	if _, err := c.ReleaseTerminal(context.Background(), acp.ReleaseTerminalRequest{}); err != nil {
		t.Fatalf("release terminal error: %v", err)
	}

	terminalOutputResp, err := c.TerminalOutput(context.Background(), acp.TerminalOutputRequest{})
	if err != nil {
		t.Fatalf("terminal output error: %v", err)
	}
	if terminalOutputResp.Output != "ok" || terminalOutputResp.Truncated {
		t.Fatalf("unexpected terminal output response: %+v", terminalOutputResp)
	}

	if _, err := c.WaitForTerminalExit(context.Background(), acp.WaitForTerminalExitRequest{}); err != nil {
		t.Fatalf("wait for terminal exit error: %v", err)
	}
}
