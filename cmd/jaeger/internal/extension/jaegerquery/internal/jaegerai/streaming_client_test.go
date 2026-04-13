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

	"github.com/coder/acp-go-sdk"
	"github.com/stretchr/testify/require"
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
	}

	c.writeAndFlush("hello")

	if got, want := rec.Body.String(), "hello"; got != want {
		t.Fatalf("unexpected body: got %q want %q", got, want)
	}
	if flusher.count != 1 {
		t.Fatalf("expected one flush, got %d", flusher.count)
	}
}

func TestStreamingClientWriteAndFlushContextDoneSetsClosedFlag(t *testing.T) {
	rec := httptest.NewRecorder()
	flusher := &countingFlusher{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	c := &streamingClient{
		requestCtx: ctx,
		w:          rec,
		flusher:    flusher,
	}

	c.writeAndFlush("ignored")

	if !c.closed {
		t.Fatal("expected client to be closed")
	}
	if got := rec.Body.String(); got != "" {
		t.Fatalf("expected empty body, got %q", got)
	}
}

func TestStreamingClientWriteAndFlushWriteErrorSetsClosedFlag(t *testing.T) {
	c := &streamingClient{
		requestCtx: context.Background(),
		w:          &errResponseWriter{},
		flusher:    &countingFlusher{},
	}

	c.writeAndFlush("hello")

	if !c.closed {
		t.Fatal("expected client to be closed on write error")
	}
}

func TestStreamingClientWriteAndFlushRecoverSetsClosedFlag(t *testing.T) {
	c := &streamingClient{
		requestCtx: context.Background(),
		w:          &panicResponseWriter{},
		flusher:    &countingFlusher{},
	}

	c.writeAndFlush("hello")

	if !c.closed {
		t.Fatal("expected client to be closed after panic")
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
	}

	c.writeAndFlush("ignored")

	if got := rec.Body.String(); got != "" {
		t.Fatalf("expected no writes when already closed, got %q", got)
	}
	if flusher.count != 0 {
		t.Fatalf("expected no flush when already closed, got %d", flusher.count)
	}
}

func TestStreamingClientRequestPermissionAlwaysDenies(t *testing.T) {
	c := &streamingClient{}

	resp, err := c.RequestPermission(context.Background(), acp.RequestPermissionRequest{})
	require.NoError(t, err)
	require.NotNil(t, resp.Outcome.Cancelled, "expected cancelled outcome when no options")

	resp, err = c.RequestPermission(context.Background(), acp.RequestPermissionRequest{
		Options: []acp.PermissionOption{{
			OptionId: "opt-1",
			Name:     "allow",
			Kind:     acp.PermissionOptionKindAllowOnce,
		}},
	})
	require.NoError(t, err)
	require.NotNil(t, resp.Outcome.Cancelled, "expected cancelled outcome even with options")
	require.Nil(t, resp.Outcome.Selected, "should never auto-approve permissions")
}

func TestStreamingClientSessionUpdate(t *testing.T) {
	rec := httptest.NewRecorder()
	flusher := &countingFlusher{}
	c := &streamingClient{
		requestCtx: context.Background(),
		w:          rec,
		flusher:    flusher,
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
}

func TestStreamingClientUtilityMethods(t *testing.T) {
	if got := valueOrUnknown(nil); got != "unknown" {
		t.Fatalf("valueOrUnknown nil mismatch: got %q", got)
	}
	status := acp.ToolCallStatusInProgress
	if got := valueOrUnknown(&status); got != "in_progress" {
		t.Fatalf("valueOrUnknown status mismatch: got %q", got)
	}
}

func TestStreamingClientUnsupportedOperationsReturnError(t *testing.T) {
	c := &streamingClient{}

	_, err := c.WriteTextFile(context.Background(), acp.WriteTextFileRequest{})
	require.ErrorIs(t, err, errNotSupported)

	_, err = c.ReadTextFile(context.Background(), acp.ReadTextFileRequest{Path: "/tmp/nope"})
	require.ErrorIs(t, err, errNotSupported)

	_, err = c.CreateTerminal(context.Background(), acp.CreateTerminalRequest{})
	require.ErrorIs(t, err, errNotSupported)

	_, err = c.KillTerminalCommand(context.Background(), acp.KillTerminalCommandRequest{})
	require.ErrorIs(t, err, errNotSupported)

	_, err = c.ReleaseTerminal(context.Background(), acp.ReleaseTerminalRequest{})
	require.ErrorIs(t, err, errNotSupported)

	_, err = c.TerminalOutput(context.Background(), acp.TerminalOutputRequest{})
	require.ErrorIs(t, err, errNotSupported)

	_, err = c.WaitForTerminalExit(context.Background(), acp.WaitForTerminalExitRequest{})
	require.ErrorIs(t, err, errNotSupported)
}
