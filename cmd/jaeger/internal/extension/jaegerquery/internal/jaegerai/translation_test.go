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
