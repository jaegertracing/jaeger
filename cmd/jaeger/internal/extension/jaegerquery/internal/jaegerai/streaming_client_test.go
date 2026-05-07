// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/coder/acp-go-sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

// parseSSEEvents extracts the JSON payloads from the `data:` frames in the
// supplied SSE body. Each returned map corresponds to one AG-UI event.
func parseSSEEvents(t *testing.T, body string) []map[string]any {
	t.Helper()
	var events []map[string]any
	for _, line := range strings.Split(body, "\n") {
		data, ok := strings.CutPrefix(line, "data: ")
		if !ok {
			continue
		}
		var event map[string]any
		require.NoError(t, json.Unmarshal([]byte(data), &event), "could not parse SSE frame %q", data)
		events = append(events, event)
	}
	return events
}

func eventTypes(events []map[string]any) []string {
	types := make([]string, 0, len(events))
	for _, e := range events {
		if t, ok := e["type"].(string); ok {
			types = append(types, t)
		}
	}
	return types
}

func TestStreamingClientWriteWritesText(t *testing.T) {
	rec := httptest.NewRecorder()
	c := &streamingClient{
		requestCtx: context.Background(),
		w:          rec,
	}

	c.write("hello")

	assert.Equal(t, "hello", rec.Body.String(), "unexpected body content")
}

func TestStreamingClientWriteContextDoneSetsClosedFlag(t *testing.T) {
	rec := httptest.NewRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	c := &streamingClient{
		requestCtx: ctx,
		w:          rec,
	}

	c.write("ignored")

	assert.True(t, c.closed, "expected client to be closed")
	assert.Empty(t, rec.Body.String(), "expected empty body")
}

func TestStreamingClientWriteErrorSetsClosedFlag(t *testing.T) {
	c := &streamingClient{
		requestCtx: context.Background(),
		w:          &errResponseWriter{},
	}

	c.write("hello")

	assert.True(t, c.closed, "expected client to be closed on write error")
}

func TestStreamingClientWriteNoopWhenClosed(t *testing.T) {
	rec := httptest.NewRecorder()
	c := &streamingClient{
		requestCtx: context.Background(),
		w:          rec,
		closed:     true,
	}

	c.write("ignored")

	assert.Empty(t, rec.Body.String(), "expected no writes when already closed")
}

func TestStreamingClientStartRunEmitsRunStarted(t *testing.T) {
	rec := httptest.NewRecorder()
	c := newStreamingClient(context.Background(), rec, "thread-1", "run-1")

	c.startRun()

	events := parseSSEEvents(t, rec.Body.String())
	require.Len(t, events, 1, "expected one RUN_STARTED event")
	assert.Equal(t, "RUN_STARTED", events[0]["type"])
	assert.Equal(t, "thread-1", events[0]["threadId"])
	assert.Equal(t, "run-1", events[0]["runId"])
	assert.Equal(t, "run-1", c.runID, "preserved runID should stay")
	assert.NotEmpty(t, c.messageID, "startRun should allocate a messageID")
}

func TestStreamingClientStartRunGeneratesIdsWhenEmpty(t *testing.T) {
	rec := httptest.NewRecorder()
	c := newStreamingClient(context.Background(), rec, "", "")

	c.startRun()

	assert.NotEmpty(t, c.threadID, "startRun should allocate a threadID when none was provided")
	assert.NotEmpty(t, c.runID, "startRun should allocate a runID when none was provided")
	assert.NotEmpty(t, c.messageID, "startRun should allocate a messageID")
}

func TestStreamingClientFinishRunClosesOpenTextAndEmitsStopReason(t *testing.T) {
	rec := httptest.NewRecorder()
	c := newStreamingClient(context.Background(), rec, "thread-1", "run-1")
	c.startRun()
	c.ensureTextStart()

	c.finishRun("end_turn")

	events := parseSSEEvents(t, rec.Body.String())
	types := eventTypes(events)
	assert.Equal(t, []string{"RUN_STARTED", "TEXT_MESSAGE_START", "TEXT_MESSAGE_END", "RUN_FINISHED"}, types)
	assert.Equal(t, "end_turn", events[3]["stopReason"])
	assert.False(t, c.textOpen, "textOpen should be reset after finishRun")
}

func TestStreamingClientFinishRunOmitsStopReasonWhenEmpty(t *testing.T) {
	rec := httptest.NewRecorder()
	c := newStreamingClient(context.Background(), rec, "thread-1", "run-1")
	c.startRun()

	c.finishRun("")

	events := parseSSEEvents(t, rec.Body.String())
	require.Len(t, events, 2)
	assert.Equal(t, "RUN_FINISHED", events[1]["type"])
	_, hasStopReason := events[1]["stopReason"]
	assert.False(t, hasStopReason, "stopReason key should be omitted when empty")
}

func TestStreamingClientFailRunEmitsRunError(t *testing.T) {
	rec := httptest.NewRecorder()
	c := newStreamingClient(context.Background(), rec, "thread-1", "run-1")
	c.startRun()
	c.ensureTextStart()

	c.failRun("boom")

	events := parseSSEEvents(t, rec.Body.String())
	types := eventTypes(events)
	assert.Equal(t, []string{"RUN_STARTED", "TEXT_MESSAGE_START", "TEXT_MESSAGE_END", "RUN_ERROR"}, types)
	assert.Equal(t, "boom", events[3]["message"])
}

func TestStreamingClientWriteSSEEventSilentlyDropsUnmarshallable(t *testing.T) {
	rec := httptest.NewRecorder()
	c := newStreamingClient(context.Background(), rec, "thread-1", "run-1")

	// Channels cannot be JSON-marshalled. writeSSEEvent must swallow the
	// error and emit nothing rather than corrupt the stream.
	c.writeSSEEvent(map[string]any{"bad": make(chan int)})

	assert.Empty(t, rec.Body.String(),
		"writeSSEEvent must not write anything when json.Marshal fails")
}

func TestStreamingClientEnsureTextStartIsIdempotent(t *testing.T) {
	rec := httptest.NewRecorder()
	c := newStreamingClient(context.Background(), rec, "thread-1", "run-1")

	c.ensureTextStart()
	c.ensureTextStart()
	c.ensureTextStart()

	types := eventTypes(parseSSEEvents(t, rec.Body.String()))
	assert.Equal(t, []string{"TEXT_MESSAGE_START"}, types, "ensureTextStart should only emit once per open message")
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

func TestStreamingClientSessionUpdateAgentMessageChunkEmitsTextContent(t *testing.T) {
	rec := httptest.NewRecorder()
	c := newStreamingClient(context.Background(), rec, "thread-1", "run-1")
	c.startRun()

	err := c.SessionUpdate(context.Background(), acp.SessionNotification{
		Update: acp.SessionUpdate{
			AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{Content: acp.TextBlock("hello world")},
		},
	})
	require.NoError(t, err)

	events := parseSSEEvents(t, rec.Body.String())
	types := eventTypes(events)
	assert.Equal(t, []string{"RUN_STARTED", "TEXT_MESSAGE_START", "TEXT_MESSAGE_CONTENT"}, types)

	contentEvent := events[2]
	assert.Equal(t, "hello world", contentEvent["delta"])
	assert.Equal(t, c.messageID, contentEvent["messageId"])
}

func TestStreamingClientSessionUpdateToolCallEmitsStartAndArgs(t *testing.T) {
	rec := httptest.NewRecorder()
	c := newStreamingClient(context.Background(), rec, "thread-1", "run-1")

	err := c.SessionUpdate(context.Background(), acp.SessionNotification{
		Update: acp.SessionUpdate{
			ToolCall: &acp.SessionUpdateToolCall{
				ToolCallId: "tool-1",
				Title:      "search traces",
				Kind:       acp.ToolKindSearch,
				RawInput:   map[string]any{"service": "checkout"},
			},
		},
	})
	require.NoError(t, err)

	events := parseSSEEvents(t, rec.Body.String())
	require.Len(t, events, 2)
	assert.Equal(t, "TOOL_CALL_START", events[0]["type"])
	assert.Equal(t, "tool-1", events[0]["toolCallId"])
	assert.Equal(t, "search traces", events[0]["title"])
	assert.Equal(t, "TOOL_CALL_ARGS", events[1]["type"])
	args, ok := events[1]["args"].(map[string]any)
	require.True(t, ok, "args should decode as object")
	assert.Equal(t, "checkout", args["service"])
}

func TestStreamingClientSessionUpdateToolCallUpdateEmitsResultAndEnd(t *testing.T) {
	rec := httptest.NewRecorder()
	c := newStreamingClient(context.Background(), rec, "thread-1", "run-1")

	completed := acp.ToolCallStatusCompleted
	err := c.SessionUpdate(context.Background(), acp.SessionNotification{
		Update: acp.SessionUpdate{
			ToolCallUpdate: &acp.SessionToolCallUpdate{
				ToolCallId: "tool-1",
				RawInput:   map[string]any{"refined": true},
				RawOutput:  map[string]any{"hits": 42},
				Status:     &completed,
			},
		},
	})
	require.NoError(t, err)

	events := parseSSEEvents(t, rec.Body.String())
	types := eventTypes(events)
	assert.Equal(t, []string{"TOOL_CALL_ARGS", "TOOL_CALL_RESULT", "TOOL_CALL_END"}, types)
	assert.Equal(t, "tool-1", events[2]["toolCallId"])
	assert.Equal(t, "completed", events[2]["status"])
}

func TestStreamingClientSessionUpdateToolCallUpdateSkipsEndForInProgress(t *testing.T) {
	rec := httptest.NewRecorder()
	c := newStreamingClient(context.Background(), rec, "thread-1", "run-1")

	inProgress := acp.ToolCallStatusInProgress
	err := c.SessionUpdate(context.Background(), acp.SessionNotification{
		Update: acp.SessionUpdate{
			ToolCallUpdate: &acp.SessionToolCallUpdate{
				ToolCallId: "tool-1",
				Status:     &inProgress,
			},
		},
	})
	require.NoError(t, err)

	assert.Empty(t, parseSSEEvents(t, rec.Body.String()),
		"in_progress tool call updates with no content should emit no SSE events")
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
