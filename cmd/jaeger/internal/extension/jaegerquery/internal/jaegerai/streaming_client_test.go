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

	aguievents "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	aguisse "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/encoding/sse"
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
// Non-`data:` framing lines (`id:`, blank separators) are ignored, matching
// how an SSE client iterates frames.
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

func TestStreamingClientEmitWritesSSEFrame(t *testing.T) {
	rec := httptest.NewRecorder()
	c := newStreamingClient(context.Background(), rec, "thread-1", "run-1")

	c.emit(aguievents.NewRunStartedEvent("thread-1", "run-1"))

	body := rec.Body.String()
	assert.Contains(t, body, "data: ", "emit should produce a data: SSE line")
	events := parseSSEEvents(t, body)
	require.Len(t, events, 1)
	assert.Equal(t, "RUN_STARTED", events[0]["type"])
}

func TestStreamingClientEmitContextDoneSetsClosedFlag(t *testing.T) {
	rec := httptest.NewRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	c := newStreamingClient(ctx, rec, "thread-1", "run-1")

	c.emit(aguievents.NewRunStartedEvent("thread-1", "run-1"))

	assert.True(t, c.closed, "expected client to be closed when context is cancelled")
	assert.Empty(t, rec.Body.String(), "expected no SSE frames after context cancellation")
}

func TestStreamingClientEmitErrorSetsClosedFlag(t *testing.T) {
	c := &streamingClient{
		requestCtx: context.Background(),
		w:          &errResponseWriter{},
		sse:        aguisse.NewSSEWriter(),
	}

	c.emit(aguievents.NewRunStartedEvent("thread-1", "run-1"))

	assert.True(t, c.closed, "expected client to be closed on write error")
}

func TestStreamingClientEmitNoopWhenClosed(t *testing.T) {
	rec := httptest.NewRecorder()
	c := &streamingClient{
		requestCtx: context.Background(),
		w:          rec,
		sse:        aguisse.NewSSEWriter(),
		closed:     true,
	}

	c.emit(aguievents.NewRunStartedEvent("thread-1", "run-1"))

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

func TestStreamingClientFinishRunClosesOpenTextAndEmitsRunFinished(t *testing.T) {
	rec := httptest.NewRecorder()
	c := newStreamingClient(context.Background(), rec, "thread-1", "run-1")
	c.startRun()
	c.ensureTextStart()

	// AG-UI's RUN_FINISHED schema has no top-level stopReason field, so
	// the sidecar's stop reason is forwarded via the schema-supported
	// `result` payload as `{"stopReason": "<reason>"}`.
	c.finishRun("end_turn")

	events := parseSSEEvents(t, rec.Body.String())
	types := eventTypes(events)
	assert.Equal(t, []string{"RUN_STARTED", "TEXT_MESSAGE_START", "TEXT_MESSAGE_END", "RUN_FINISHED"}, types)
	finishedEvent := events[3]
	_, hasStopReasonAtRoot := finishedEvent["stopReason"]
	assert.False(t, hasStopReasonAtRoot,
		"stopReason must not be a top-level field — the AG-UI schema forbids extras at that level")
	result, ok := finishedEvent["result"].(map[string]any)
	require.True(t, ok, "stopReason must ride inside the schema-supported result payload")
	assert.Equal(t, "end_turn", result["stopReason"])
	assert.False(t, c.textOpen, "textOpen should be reset after finishRun")
}

func TestStreamingClientFinishRunOmitsResultWhenNoStopReason(t *testing.T) {
	rec := httptest.NewRecorder()
	c := newStreamingClient(context.Background(), rec, "thread-1", "run-1")
	c.startRun()

	c.finishRun("")

	events := parseSSEEvents(t, rec.Body.String())
	require.Len(t, events, 2)
	finishedEvent := events[1]
	assert.Equal(t, "RUN_FINISHED", finishedEvent["type"])
	_, hasResult := finishedEvent["result"]
	assert.False(t, hasResult,
		"result must be omitted when no stop reason is supplied so the event stays compact")
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
	assert.Equal(t, "run-1", events[3]["runId"], "RUN_ERROR should propagate runId so the frontend can correlate")
}

func TestStreamingClientEnsureTextStartIsIdempotent(t *testing.T) {
	rec := httptest.NewRecorder()
	c := newStreamingClient(context.Background(), rec, "thread-1", "run-1")
	// startRun would normally allocate this; set it directly so the test
	// targets ensureTextStart only. Without a messageID the SDK rejects the
	// event at encode time and emits nothing.
	c.messageID = "msg-1"

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
				Title:      "search_traces",
				Kind:       acp.ToolKindSearch,
				RawInput:   map[string]any{"service": "checkout"},
			},
		},
	})
	require.NoError(t, err)

	events := parseSSEEvents(t, rec.Body.String())
	require.Len(t, events, 2)

	startEvent := events[0]
	assert.Equal(t, "TOOL_CALL_START", startEvent["type"])
	assert.Equal(t, "tool-1", startEvent["toolCallId"])
	assert.Equal(t, "search_traces", startEvent["toolCallName"],
		"AG-UI TOOL_CALL_START requires toolCallName")

	argsEvent := events[1]
	assert.Equal(t, "TOOL_CALL_ARGS", argsEvent["type"])
	assert.Equal(t, "tool-1", argsEvent["toolCallId"])
	// AG-UI streams args as text deltas; the whole input is emitted as one
	// JSON-encoded delta because the sidecar delivers args atomically.
	delta, ok := argsEvent["delta"].(string)
	require.True(t, ok, "TOOL_CALL_ARGS.delta must be a string")
	assert.JSONEq(t, `{"service":"checkout"}`, delta,
		"TOOL_CALL_ARGS.delta must be the JSON-encoded args string")
}

func TestStreamingClientSessionUpdateToolCallStripsUIPrefixForName(t *testing.T) {
	rec := httptest.NewRecorder()
	c := newStreamingClient(context.Background(), rec, "thread-1", "run-1")

	// Contextual UI tools are registered with the LLM under a UIToolPrefix
	// namespace so they never collide with built-in MCP tool names. The
	// frontend, however, registered them under their unprefixed names —
	// TOOL_CALL_START.toolCallName must be stripped so it round-trips.
	err := c.SessionUpdate(context.Background(), acp.SessionNotification{
		Update: acp.SessionUpdate{
			ToolCall: &acp.SessionUpdateToolCall{
				ToolCallId: "tool-2",
				Title:      UIToolPrefix + "render_chart",
				Kind:       acp.ToolKindOther,
			},
		},
	})
	require.NoError(t, err)

	events := parseSSEEvents(t, rec.Body.String())
	require.Len(t, events, 1)
	assert.Equal(t, "render_chart", events[0]["toolCallName"],
		"contextual tool names must have %q stripped before being sent to the frontend", UIToolPrefix)
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

	argsEvent := events[0]
	assert.Equal(t, `{"refined":true}`, argsEvent["delta"])

	resultEvent := events[1]
	assert.Equal(t, "tool-1", resultEvent["toolCallId"])
	assert.Equal(t, "tool-msg-tool-1", resultEvent["messageId"],
		"AG-UI TOOL_CALL_RESULT requires messageId")
	assert.Equal(t, "tool", resultEvent["role"])
	assert.Equal(t, `{"hits":42}`, resultEvent["content"],
		"non-MCP-envelope outputs should be JSON-encoded into the content string")
	_, hasResult := resultEvent["result"]
	assert.False(t, hasResult, "legacy nested result field must not be present")

	assert.Equal(t, "tool-1", events[2]["toolCallId"])
}

func TestStreamingClientSessionUpdateToolCallUpdateFlattensMCPEnvelope(t *testing.T) {
	rec := httptest.NewRecorder()
	c := newStreamingClient(context.Background(), rec, "thread-1", "run-1")

	completed := acp.ToolCallStatusCompleted
	// Sidecar forwards MCP CallToolResult envelopes verbatim. The handler
	// must concatenate the text blocks so AG-UI receives content as a string.
	err := c.SessionUpdate(context.Background(), acp.SessionNotification{
		Update: acp.SessionUpdate{
			ToolCallUpdate: &acp.SessionToolCallUpdate{
				ToolCallId: "search_traces-1",
				RawOutput: map[string]any{
					"content": []any{
						map[string]any{"type": "text", "text": `{"traces":[]}`},
					},
					"isError":           false,
					"structuredContent": map[string]any{"traces": []any{}},
				},
				Status: &completed,
			},
		},
	})
	require.NoError(t, err)

	events := parseSSEEvents(t, rec.Body.String())
	require.Len(t, events, 2, "expected TOOL_CALL_RESULT and TOOL_CALL_END")

	resultEvent := events[0]
	assert.Equal(t, "TOOL_CALL_RESULT", resultEvent["type"])
	content, ok := resultEvent["content"].(string)
	require.True(t, ok, "TOOL_CALL_RESULT.content must be a string")
	assert.JSONEq(t, `{"traces":[]}`, content,
		"MCP text block should become the top-level content string")
	assert.Equal(t, "tool-msg-search_traces-1", resultEvent["messageId"])
	assert.Equal(t, "tool", resultEvent["role"])
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

	_, err = c.KillTerminal(context.Background(), acp.KillTerminalRequest{})
	require.ErrorIs(t, err, errNotSupported)

	_, err = c.ReleaseTerminal(context.Background(), acp.ReleaseTerminalRequest{})
	require.ErrorIs(t, err, errNotSupported)

	_, err = c.TerminalOutput(context.Background(), acp.TerminalOutputRequest{})
	require.ErrorIs(t, err, errNotSupported)

	_, err = c.WaitForTerminalExit(context.Background(), acp.WaitForTerminalExitRequest{})
	require.ErrorIs(t, err, errNotSupported)
}
