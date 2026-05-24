// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"encoding/json"
	"testing"

	aguitypes "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLatestUserMessageTextPicksMostRecentUser(t *testing.T) {
	messages := []aguitypes.Message{
		{Role: aguitypes.RoleUser, Content: "older user message"},
		{Role: "assistant", Content: "assistant reply"},
		{Role: aguitypes.RoleUser, Content: "newest user message"},
	}

	text, err := latestUserMessageText(messages)
	require.NoError(t, err)
	assert.Equal(t, "newest user message", text)
}

func TestLatestUserMessageTextReturnsErrorWhenNoUserMessages(t *testing.T) {
	_, err := latestUserMessageText([]aguitypes.Message{{Role: "assistant", Content: "hi"}})
	require.Error(t, err)
}

func TestLatestUserMessageTextSkipsBlankUserMessages(t *testing.T) {
	_, err := latestUserMessageText([]aguitypes.Message{{Role: aguitypes.RoleUser, Content: "   "}})
	require.Error(t, err, "messages with only whitespace must be treated as empty")
}

func TestLatestUserMessageTextSupportsInputContentParts(t *testing.T) {
	parts := []aguitypes.InputContent{
		{Type: aguitypes.InputContentTypeText, Text: "first"},
		{Type: aguitypes.InputContentTypeText, Text: "second"},
	}
	text, err := latestUserMessageText([]aguitypes.Message{{Role: aguitypes.RoleUser, Content: parts}})
	require.NoError(t, err)
	assert.Equal(t, "first\nsecond", text)
}

func TestContextTextEntriesFormatsDescriptionAndValue(t *testing.T) {
	entries := []aguitypes.Context{
		{Description: "trace", Value: "abc"},
		{Value: "only value"},
		{Value: "   "},
	}
	got := contextTextEntries(entries)
	assert.Equal(t, []string{"trace:\nabc", "only value"}, got,
		"blank values should be filtered out, descriptions prepended when present")
}

func TestEncodeToolsAsRawMarshalsEachTool(t *testing.T) {
	tools := []aguitypes.Tool{
		{Name: "tool-a", Description: "first", Parameters: map[string]any{"type": "object"}},
		{Name: "tool-b"},
	}
	raw := encodeToolsAsRaw(tools)
	require.Len(t, raw, 2)

	var first map[string]any
	require.NoError(t, json.Unmarshal(raw[0], &first))
	assert.Equal(t, "tool-a", first["name"])
	assert.Equal(t, "first", first["description"])
}

func TestExtractTextHandlesStringBytesAndStructuredContent(t *testing.T) {
	assert.Equal(t, "plain", extractText("plain"))
	assert.Empty(t, extractText(nil))

	parts := []aguitypes.InputContent{{Type: aguitypes.InputContentTypeText, Text: "typed"}}
	assert.Equal(t, "typed", extractText(parts))

	// JSON-decoded parts (any) path.
	decoded := []any{
		map[string]any{"type": "text", "text": "part-1"},
		"bare-string",
	}
	assert.Equal(t, "part-1\nbare-string", extractText(decoded))

	// Object with top-level text field.
	assert.Equal(t, "direct", extractText(map[string]any{"text": "direct"}))

	// Object with parts array.
	assert.Equal(t, "fragment", extractText(map[string]any{
		"parts": []any{map[string]any{"text": "fragment"}},
	}))
}

func TestExtractTextReturnsEmptyForUnknownShape(t *testing.T) {
	assert.Empty(t, extractText(42))
}

func TestEncodeToolsAsRawSkipsUnmarshalableTool(t *testing.T) {
	// A chan is not JSON-marshalable, so the middle tool should be dropped
	// while the neighbours are still encoded.
	tools := []aguitypes.Tool{
		{Name: "ok-1"},
		{Name: "broken", Parameters: map[string]any{"ch": make(chan int)}},
		{Name: "ok-2"},
	}
	raw := encodeToolsAsRaw(tools)
	require.Len(t, raw, 2)

	var first, second map[string]any
	require.NoError(t, json.Unmarshal(raw[0], &first))
	require.NoError(t, json.Unmarshal(raw[1], &second))
	assert.Equal(t, "ok-1", first["name"])
	assert.Equal(t, "ok-2", second["name"])
}

func TestExtractTextReturnsEmptyWhenMarshalFails(t *testing.T) {
	// A channel cannot be JSON-marshalled, which exercises the error path
	// after the string and []InputContent type assertions both miss.
	assert.Empty(t, extractText(make(chan int)))
}

func TestExtractTextDecodesMarshalledInputContent(t *testing.T) {
	// []map[string]any is not the concrete []aguitypes.InputContent type, so
	// extractText falls through to json.Marshal and then decodeInputContent
	// before collecting the text parts.
	content := []map[string]any{
		{"type": aguitypes.InputContentTypeText, "text": "hello"},
		{"type": aguitypes.InputContentTypeText, "text": "world"},
	}
	assert.Equal(t, "hello\nworld", extractText(content))
}

func TestCollectInputContentTextIgnoresNonTextParts(t *testing.T) {
	parts := []aguitypes.InputContent{
		{Type: aguitypes.InputContentTypeText, Text: "keep"},
		{Type: aguitypes.InputContentTypeBinary, MimeType: "image/png", URL: "https://example.com/x.png"},
		{Type: aguitypes.InputContentTypeText, Text: "also-keep"},
	}
	assert.Equal(t, "keep\nalso-keep", collectInputContentText(parts))
}

func TestExtractTextDecodesDirectStringFromMarshaledBytes(t *testing.T) {
	// json.RawMessage is []byte, so the string type assertion misses but the
	// marshaled output is a JSON string that falls through to the direct
	// string unmarshal branch.
	raw := json.RawMessage(`"  trimmed  "`)
	assert.Equal(t, "trimmed", extractText(raw))
}

func TestCollectTextPartsIgnoresUnsupportedElementTypes(t *testing.T) {
	parts := []any{
		"keep",
		42,
		true,
		map[string]any{"text": "also-keep"},
	}
	assert.Equal(t, "keep\nalso-keep", collectTextParts(parts))
}

func TestMarshalToolArgsDeltaJSONEncodes(t *testing.T) {
	// Happy path: a JSON-marshallable value should round-trip through
	// json.Marshal and become a compact JSON string for AG-UI's delta field.
	delta := marshalToolArgsDelta(map[string]any{"service": "checkout", "limit": 10})
	assert.JSONEq(t, `{"service":"checkout","limit":10}`, delta)
}

func TestMarshalToolArgsDeltaFallsBackOnMarshalError(t *testing.T) {
	// channels cannot be JSON-marshalled. Rather than emit an empty delta
	// (which the SDK would reject for failing the Validate step), the
	// helper falls back to fmt.Sprintf so the frontend at least sees
	// something — better than dropping the entire TOOL_CALL_ARGS frame.
	delta := marshalToolArgsDelta(make(chan int))
	assert.NotEmpty(t, delta,
		"fallback must produce a non-empty string when json.Marshal fails — "+
			"otherwise the SDK's Validate step rejects the event and the frame is dropped")
}

func TestFlattenToolResultContentJoinsMultipleTextBlocksWithNewline(t *testing.T) {
	// MCP CallToolResult envelopes can carry several text blocks — agents
	// often emit one block per paragraph/segment. Concatenating with no
	// delimiter would mash distinct lines into a run-on string and lose
	// the agent's intended structure; joining with "\n" preserves block
	// boundaries while still satisfying AG-UI's single-string contract.
	envelope := map[string]any{
		"content": []any{
			map[string]any{"type": "text", "text": "Found 3 services."},
			map[string]any{"type": "text", "text": "Top latency: 1.2s on checkout."},
		},
	}
	got := flattenToolResultContent(envelope)
	assert.Equal(t, "Found 3 services.\nTop latency: 1.2s on checkout.", got,
		"multiple MCP text blocks must be separated by a newline so frontends can render them as distinct lines")
}

func TestFlattenToolResultContentPassesSingleBlockThrough(t *testing.T) {
	// A single text block emits exactly that text — no leading/trailing
	// newline. This is the common single-paragraph case and ensures the
	// join helper doesn't add a separator when there's nothing to separate.
	envelope := map[string]any{
		"content": []any{
			map[string]any{"type": "text", "text": `{"traces":[]}`},
		},
	}
	got := flattenToolResultContent(envelope)
	assert.JSONEq(t, `{"traces":[]}`, got,
		"a single block must surface verbatim with no surrounding delimiters")
}

func TestFlattenToolResultContentSkipsNonMapBlocks(t *testing.T) {
	// MCP envelopes are well-typed in practice, but the loop guards
	// against stray non-map entries (e.g. a bare string sneaking into
	// content[]) by skipping them. With no usable text blocks the helper
	// falls through to JSON-encoding the whole envelope so the frontend
	// still gets a deterministic string.
	envelope := map[string]any{
		"content": []any{
			"a bare string that should be skipped",
			42, // also not a map[string]any
		},
	}
	got := flattenToolResultContent(envelope)
	assert.JSONEq(t, `{"content":["a bare string that should be skipped",42]}`, got,
		"with no concatenable text blocks the helper should JSON-encode the whole envelope")
}

func TestFlattenToolResultContentFallsBackOnMarshalError(t *testing.T) {
	// Non-envelope inputs go straight to json.Marshal. When that fails
	// (e.g. channels) the helper renders the value via fmt.Sprintf so the
	// AG-UI TOOL_CALL_RESULT.content field is never empty — an empty
	// content string would fail the SDK's Validate step and silently drop
	// the frame.
	got := flattenToolResultContent(make(chan int))
	assert.NotEmpty(t, got,
		"fallback must produce a non-empty string when json.Marshal fails — "+
			"otherwise the SDK's Validate step rejects the event and the frame is dropped")
}

func TestStripUIToolPrefixStripsRealName(t *testing.T) {
	assert.Equal(t, "render_chart", stripUIToolPrefix(UIToolPrefix+"render_chart"))
}

func TestStripUIToolPrefixPassesThroughUnprefixed(t *testing.T) {
	assert.Equal(t, "search_traces", stripUIToolPrefix("search_traces"),
		"built-in MCP tool names must not be touched")
}

func TestStripUIToolPrefixFallsBackOnEmptyStrip(t *testing.T) {
	// An input of exactly the prefix would strip to "", which AG-UI tool-call
	// events reject and which would break SSE encoding. The streaming path
	// must defend itself even though handleJaegerToolCall already rejects
	// this shape on the ext_method side — the streaming client receives ACP
	// session/update notifications independently, so a malformed upstream
	// name must not terminate the run.
	assert.Equal(t, UIToolPrefix, stripUIToolPrefix(UIToolPrefix),
		"a name that is exactly the prefix must NOT strip to empty — fall back to the original")
}

func TestValidateContextualToolNamesAcceptsEmptyAndValidSlices(t *testing.T) {
	// A request with no tools is the common case (no contextual tools
	// declared) — validate must accept it without trying to read the
	// nil slice. An all-valid slice must also pass through cleanly so
	// the happy path is never gated on this check.
	assert.NoError(t, validateContextualToolNames(nil),
		"nil/empty tool list must not be flagged")
	assert.NoError(t, validateContextualToolNames([]aguitypes.Tool{
		{Name: "render_chart"},
		{Name: "show_flamegraph", Description: "open flamegraph view"},
	}), "an all-valid slice must pass validation")
}

func TestValidateContextualToolNamesRejectsEmptyName(t *testing.T) {
	err := validateContextualToolNames([]aguitypes.Tool{{Name: ""}})
	require.Error(t, err, "empty name must be rejected — would prefix to a bare 'ui_'")
	assert.Contains(t, err.Error(), "tools[0].name")
}

func TestValidateContextualToolNamesRejectsWhitespaceName(t *testing.T) {
	// strings.TrimSpace covers ASCII spaces, tabs, newlines, and the
	// other Unicode whitespace categories — any of which would slip past
	// a naive `name == ""` check and produce 'ui_   ' style entries
	// downstream.
	err := validateContextualToolNames([]aguitypes.Tool{{Name: "  \t\n  "}})
	require.Error(t, err, "whitespace-only name must be rejected")
	assert.Contains(t, err.Error(), "tools[0].name")
}

func TestValidateContextualToolNamesReportsFirstOffendingIndex(t *testing.T) {
	// The error message must point at the first bad entry's index, not
	// just say "some tool was invalid" — that's what makes the 400
	// useful to a frontend developer debugging a typo in the tools array.
	err := validateContextualToolNames([]aguitypes.Tool{
		{Name: "ok-first"},
		{Name: "ok-second"},
		{Name: ""},
		{Name: "would-also-be-bad"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tools[2].name",
		"the index of the first invalid tool must be in the error so frontend devs can locate it")
}
