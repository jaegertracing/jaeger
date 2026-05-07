// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/coder/acp-go-sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type flushCountingRecorder struct {
	httptest.ResponseRecorder
	count int
}

func newFlushCountingRecorder() *flushCountingRecorder {
	return &flushCountingRecorder{
		ResponseRecorder: *httptest.NewRecorder(),
	}
}

func (f *flushCountingRecorder) Flush() {
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
	rec := newFlushCountingRecorder()
	c := &streamingClient{
		requestCtx: context.Background(),
		w:          rec,
	}

	c.writeAndFlush("hello")

	assert.Equal(t, "hello", rec.Body.String(), "unexpected body content")
	assert.Equal(t, 1, rec.count, "expected one flush")
}

func TestStreamingClientWriteAndFlushContextDoneSetsClosedFlag(t *testing.T) {
	rec := newFlushCountingRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	c := &streamingClient{
		requestCtx: ctx,
		w:          rec,
	}

	c.writeAndFlush("ignored")

	assert.True(t, c.closed, "expected client to be closed")
	assert.Empty(t, rec.Body.String(), "expected empty body")
}

func TestStreamingClientWriteAndFlushWriteErrorSetsClosedFlag(t *testing.T) {
	c := &streamingClient{
		requestCtx: context.Background(),
		w:          &errResponseWriter{},
	}

	c.writeAndFlush("hello")

	assert.True(t, c.closed, "expected client to be closed on write error")
}

func TestStreamingClientWriteAndFlushRecoverSetsClosedFlag(t *testing.T) {
	c := &streamingClient{
		requestCtx: context.Background(),
		w:          &panicResponseWriter{},
	}

	c.writeAndFlush("hello")

	assert.True(t, c.closed, "expected client to be closed after panic")
}

func TestStreamingClientWriteAndFlushNoopWhenClosed(t *testing.T) {
	rec := newFlushCountingRecorder()
	c := &streamingClient{
		requestCtx: context.Background(),
		w:          rec,
		closed:     true,
	}

	c.writeAndFlush("ignored")

	assert.Empty(t, rec.Body.String(), "expected no writes when already closed")
	assert.Zero(t, rec.count, "expected no flush when already closed")
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
	rec := newFlushCountingRecorder()
	c := &streamingClient{
		requestCtx: context.Background(),
		w:          rec,
	}

	status := acp.ToolCallStatusCompleted
	err := c.SessionUpdate(context.Background(), acp.SessionNotification{
		Update: acp.SessionUpdate{
			AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{Content: acp.TextBlock("chunk")},
			ToolCall:          &acp.SessionUpdateToolCall{Title: "search traces"},
			ToolCallUpdate:    &acp.SessionToolCallUpdate{ToolCallId: "tool-1", Status: &status},
		},
	})
	require.NoError(t, err)

	got := rec.Body.String()
	assert.Contains(t, got, "chunk")
	assert.Contains(t, got, "[tool_call] search traces")
	assert.Contains(t, got, "[tool_result] id=tool-1 status=completed")
}

func TestStreamingClientUtilityMethods(t *testing.T) {
	assert.Equal(t, "unknown", valueOrUnknown(nil))
	status := acp.ToolCallStatusInProgress
	assert.Equal(t, "in_progress", valueOrUnknown(&status))
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
