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
//
// Per the SSE spec the colon may be followed by one optional space; both
// `data: {...}` and `data:{...}` are valid frames a server may emit. We
// accept either so the tests do not regress if the AG-UI SDK ever changes
// its framing whitespace.
func parseSSEEvents(t *testing.T, body string) []map[string]any {
	t.Helper()
	var events []map[string]any
	for _, line := range strings.Split(body, "\n") {
		data, ok := strings.CutPrefix(line, "data:")
		if !ok {
			continue
		}
		// The SSE spec allows exactly one optional space after `data:`
		// — strip it if present so the JSON parser sees the payload
		// regardless of which framing variant the writer chose.
		data = strings.TrimPrefix(data, " ")
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

func TestParseSSEEventsAcceptsDataPrefixWithOrWithoutSpace(t *testing.T) {
	// Per the SSE spec, the colon may be followed by exactly one optional
	// space. AG-UI's SDK writer currently emits "data: {...}\n\n" but the
	// parser must tolerate the no-space variant so the tests do not
	// regress if the SDK ever switches framing whitespace.
	body := "data: {\"type\":\"RUN_STARTED\",\"runId\":\"r1\"}\n\n" +
		"data:{\"type\":\"RUN_FINISHED\",\"runId\":\"r2\"}\n\n"

	events := parseSSEEvents(t, body)
	require.Len(t, events, 2,
		"both `data:` and `data: ` framing variants must be recognised by the parser")
	assert.Equal(t, "RUN_STARTED", events[0]["type"])
	assert.Equal(t, "RUN_FINISHED", events[1]["type"])
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

	// Pin the single-nanosecond stem: when all three IDs are allocated by
	// the same startRun call, they must share the suffix so a debugger
	// can see at a glance they came from the same run. Independent
	// time.Now() calls could produce identical values on coarse-clock
	// systems and unrelated-looking values on fast clocks; sharing the
	// stem covers both ends.
	threadSuffix := strings.TrimPrefix(c.threadID, "thread-")
	runSuffix := strings.TrimPrefix(c.runID, "run-")
	msgSuffix := strings.TrimPrefix(c.messageID, "msg-")
	assert.Equal(t, threadSuffix, runSuffix, "threadID and runID must share the nanosecond stem")
	assert.Equal(t, threadSuffix, msgSuffix, "threadID and messageID must share the nanosecond stem")
}

func TestStreamingClientStartRunMintsUniqueStemsAcrossCalls(t *testing.T) {
	// The process-wide atomic counter is the guarantee against coarse-clock
	// collisions: two streamingClients constructed and startRun-ed in the
	// same nanosecond must still get distinct stems. Calling startRun
	// twice in rapid succession in a single goroutine is the simplest way
	// to exercise that — the timestamp may or may not increment between
	// calls, but the counter must.
	a := newStreamingClient(context.Background(), httptest.NewRecorder(), "", "")
	b := newStreamingClient(context.Background(), httptest.NewRecorder(), "", "")

	a.startRun()
	b.startRun()

	assert.NotEqual(t, a.threadID, b.threadID,
		"two consecutive startRuns must produce different stems even on coarse-clock systems — the seq counter is the safety net")
	assert.NotEqual(t, a.runID, b.runID, "runIDs must be distinct too")
	assert.NotEqual(t, a.messageID, b.messageID, "messageIDs must be distinct too")
}

func TestStreamingClientStartRunDoesNotOverwriteSuppliedIDs(t *testing.T) {
	// When the caller already supplies threadID/runID, startRun must keep
	// them as-is and only allocate the missing messageID. The caller's
	// IDs come from the AG-UI RunAgentInput and are what the frontend
	// uses to correlate the run — overwriting them would orphan events.
	rec := httptest.NewRecorder()
	c := newStreamingClient(context.Background(), rec, "thread-from-frontend", "run-from-frontend")

	c.startRun()

	assert.Equal(t, "thread-from-frontend", c.threadID, "caller-supplied threadID must survive startRun")
	assert.Equal(t, "run-from-frontend", c.runID, "caller-supplied runID must survive startRun")
	assert.NotEmpty(t, c.messageID, "missing messageID must still be allocated")
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

func TestStreamingClientRequestPermissionAcceptsAllowOnce(t *testing.T) {
	// Agents like the Claude Agent SDK route every tool call through a
	// session/request_permission RPC. The gateway already curates which
	// tools the agent can invoke (operator-configured MCP + frontend-
	// declared contextual tools), and AG-UI has no per-call permission
	// event to bubble up — so the gateway must select an allow option
	// rather than cancel, which Claude treats as "Tool use aborted".
	c := &streamingClient{}

	resp, err := c.RequestPermission(context.Background(), acp.RequestPermissionRequest{
		Options: []acp.PermissionOption{{
			OptionId: "opt-1",
			Name:     "allow once",
			Kind:     acp.PermissionOptionKindAllowOnce,
		}},
	})
	require.NoError(t, err)
	require.NotNil(t, resp.Outcome.Selected, "expected allow-kind option to be selected")
	require.Nil(t, resp.Outcome.Cancelled, "must not cancel when an allow option is offered")
	assert.Equal(t, acp.PermissionOptionId("opt-1"), resp.Outcome.Selected.OptionId)
}

func TestStreamingClientRequestPermissionPicksFirstAllowOption(t *testing.T) {
	// Agents typically present a list like [allow_once, allow_always,
	// reject_once]. Order-preserving "first allow wins" keeps the agent's
	// suggested ordering authoritative — the gateway has no preference
	// between scopes, so don't impose one.
	c := &streamingClient{}

	resp, err := c.RequestPermission(context.Background(), acp.RequestPermissionRequest{
		Options: []acp.PermissionOption{
			{OptionId: "reject", Name: "reject", Kind: acp.PermissionOptionKindRejectOnce},
			{OptionId: "once", Name: "allow once", Kind: acp.PermissionOptionKindAllowOnce},
			{OptionId: "always", Name: "allow always", Kind: acp.PermissionOptionKindAllowAlways},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp.Outcome.Selected)
	assert.Equal(t, acp.PermissionOptionId("once"), resp.Outcome.Selected.OptionId,
		"first allow-kind option must be chosen, skipping any preceding reject options")
}

func TestStreamingClientRequestPermissionAcceptsAllowAlwaysWhenNoOnce(t *testing.T) {
	// Some agents may only offer allow_always (e.g. for tools they consider
	// safe to remember). Accept that too — both kinds are "yes."
	c := &streamingClient{}

	resp, err := c.RequestPermission(context.Background(), acp.RequestPermissionRequest{
		Options: []acp.PermissionOption{{
			OptionId: "always",
			Name:     "allow always",
			Kind:     acp.PermissionOptionKindAllowAlways,
		}},
	})
	require.NoError(t, err)
	require.NotNil(t, resp.Outcome.Selected)
	assert.Equal(t, acp.PermissionOptionId("always"), resp.Outcome.Selected.OptionId)
}

func TestStreamingClientRequestPermissionCancelsWhenNoOptions(t *testing.T) {
	// Defensive: with no options to choose from there's nothing to accept,
	// so fall through to cancelled rather than fabricate an option id.
	c := &streamingClient{}

	resp, err := c.RequestPermission(context.Background(), acp.RequestPermissionRequest{})
	require.NoError(t, err)
	require.NotNil(t, resp.Outcome.Cancelled, "no options → cancelled")
	require.Nil(t, resp.Outcome.Selected)
}

func TestStreamingClientRequestPermissionCancelsWhenOnlyRejectOptions(t *testing.T) {
	// A misconfigured or hostile agent that only presents reject options
	// cannot be coerced into an allow — the gateway falls through to
	// cancelled so the tool call is aborted instead of forced.
	c := &streamingClient{}

	resp, err := c.RequestPermission(context.Background(), acp.RequestPermissionRequest{
		Options: []acp.PermissionOption{
			{OptionId: "reject-once", Kind: acp.PermissionOptionKindRejectOnce},
			{OptionId: "reject-always", Kind: acp.PermissionOptionKindRejectAlways},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp.Outcome.Cancelled)
	require.Nil(t, resp.Outcome.Selected, "must not promote a reject option to an allow")
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

func TestStreamingClientSessionUpdateAgentMessageChunkSkipsEmptyText(t *testing.T) {
	// AG-UI's TextMessageContentEvent forbids empty deltas — its SDK encoder
	// rejects them at write time, which would mark the SSE stream closed and
	// strand the browser bubble after TEXT_MESSAGE_START. ACP itself permits
	// empty-text chunks (Claude maps content_block_start lifecycle events
	// that way), so the gateway must filter them at the boundary. No
	// TEXT_MESSAGE_START fires either: pairing START with content that
	// never arrives breaks the AG-UI invariant START → CONTENT+ → END.
	rec := httptest.NewRecorder()
	c := newStreamingClient(context.Background(), rec, "thread-1", "run-1")

	err := c.SessionUpdate(context.Background(), acp.SessionNotification{
		Update: acp.SessionUpdate{
			AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{Content: acp.TextBlock("")},
		},
	})
	require.NoError(t, err)
	require.Empty(t, rec.Body.String(),
		"empty-text agent_message_chunk must not produce any SSE events")
	assert.False(t, c.textOpen,
		"TEXT_MESSAGE_START must not be emitted when no content will follow")
}

func TestStreamingClientSessionUpdateAgentMessageChunkSkipsWhitespaceOnlyText(t *testing.T) {
	// Some agents emit framing chunks containing only whitespace (e.g. a
	// newline marking a block boundary). The AG-UI encoder treats those the
	// same as empty deltas, so they get filtered out at the boundary
	// alongside truly-empty chunks.
	for _, blank := range []string{" ", "  \t  ", "\n", "\r\n"} {
		t.Run(strings.ReplaceAll(strings.ReplaceAll(blank, "\n", "\\n"), "\t", "\\t"), func(t *testing.T) {
			rec := httptest.NewRecorder()
			c := newStreamingClient(context.Background(), rec, "thread-1", "run-1")

			err := c.SessionUpdate(context.Background(), acp.SessionNotification{
				Update: acp.SessionUpdate{
					AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{Content: acp.TextBlock(blank)},
				},
			})
			require.NoError(t, err)
			require.Empty(t, rec.Body.String(),
				"whitespace-only agent_message_chunk %q must not produce any SSE events", blank)
			assert.False(t, c.textOpen,
				"TEXT_MESSAGE_START must not be emitted for whitespace-only chunks")
		})
	}
}

func TestStreamingClientSessionUpdateAgentMessageChunkPreservesInternalWhitespace(t *testing.T) {
	// Whitespace inside a non-blank chunk is meaningful glue between
	// streamed deltas — e.g. when an agent emits "Hello" then " world"
	// separately, the leading space must survive so the assembled message
	// reads "Hello world". The filter only drops fully-blank chunks; it
	// does NOT trim the delta when it forwards.
	rec := httptest.NewRecorder()
	c := newStreamingClient(context.Background(), rec, "thread-1", "run-1")
	c.startRun()

	err := c.SessionUpdate(context.Background(), acp.SessionNotification{
		Update: acp.SessionUpdate{
			AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{Content: acp.TextBlock(" world ")},
		},
	})
	require.NoError(t, err)

	events := parseSSEEvents(t, rec.Body.String())
	require.Equal(t,
		[]string{"RUN_STARTED", "TEXT_MESSAGE_START", "TEXT_MESSAGE_CONTENT"},
		eventTypes(events))
	assert.Equal(t, " world ", events[2]["delta"],
		"non-blank chunks must be forwarded verbatim, preserving leading/trailing whitespace")
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

func TestStreamingClientEmitContextualToolCallEmitsStartArgsEnd(t *testing.T) {
	// The MCP proxy calls EmitContextualToolCall whenever an agent
	// invokes a UI tool via /api/ai/mcp/.../tools/call. The contract:
	// emit START + ARGS + END for the same toolCallId, NEVER RESULT.
	// (RESULT would short-circuit assistant-ui — the browser is the
	// executor.) Args carried verbatim as the delta string.
	rec := httptest.NewRecorder()
	c := newStreamingClient(context.Background(), rec, "thread-1", "run-1")

	c.EmitContextualToolCall("call-id-1", "ui_highlight_span", map[string]any{
		"spanId": "abc123",
	})

	events := parseSSEEvents(t, rec.Body.String())
	types := eventTypes(events)
	assert.Equal(t,
		[]string{"TOOL_CALL_START", "TOOL_CALL_ARGS", "TOOL_CALL_END"},
		types,
		"the MCP-proxy UI-tool dispatch must emit exactly START + ARGS + END with no RESULT in between")

	startEvt := events[0]
	assert.Equal(t, "call-id-1", startEvt["toolCallId"])
	assert.Equal(t, "ui_highlight_span", startEvt["toolCallName"])

	argsEvt := events[1]
	assert.Equal(t, "call-id-1", argsEvt["toolCallId"])
	delta, ok := argsEvt["delta"].(string)
	require.True(t, ok, "TOOL_CALL_ARGS.delta must be a string")
	assert.JSONEq(t, `{"spanId":"abc123"}`, delta)

	endEvt := events[2]
	assert.Equal(t, "call-id-1", endEvt["toolCallId"])
}

func TestStreamingClientEmitContextualToolCallSkipsArgsForNilPayload(t *testing.T) {
	// An MCP tool with an empty `{}` parameters block can legitimately
	// arrive with no `Arguments` field (the agent's MCP client elides
	// it). The browser doesn't need an ARGS event in that case — START
	// + END is enough to drive assistant-ui's local executor. Skipping
	// the empty event keeps the SSE stream uncluttered and matches the
	// ACP-driven SessionUpdate path, which also conditions ARGS on a
	// non-nil RawInput.
	rec := httptest.NewRecorder()
	c := newStreamingClient(context.Background(), rec, "thread-1", "run-1")

	c.EmitContextualToolCall("call-id-2", "ui_clear_filters", nil)

	events := parseSSEEvents(t, rec.Body.String())
	types := eventTypes(events)
	assert.Equal(t, []string{"TOOL_CALL_START", "TOOL_CALL_END"}, types,
		"nil args means no TOOL_CALL_ARGS event; START and END still bracket the call")
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
