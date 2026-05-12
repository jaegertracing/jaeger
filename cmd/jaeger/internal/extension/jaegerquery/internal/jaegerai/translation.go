// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	aguitypes "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
	"github.com/coder/acp-go-sdk"
)

// latestUserMessageText returns the text content of the most recent user
// message in the AG-UI run input. It is an error if no user message carries
// any textual content.
func latestUserMessageText(messages []aguitypes.Message) (string, error) {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != aguitypes.RoleUser {
			continue
		}
		text := extractText(messages[i].Content)
		if text != "" {
			return text, nil
		}
	}
	return "", errors.New("no user message with text content found")
}

// contextTextEntries flattens AG-UI context entries into strings that can be
// appended as extra prompt blocks. An entry with a description is rendered as
// "description:\nvalue".
func contextTextEntries(entries []aguitypes.Context) []string {
	result := make([]string, 0, len(entries))
	for _, entry := range entries {
		if text := strings.TrimSpace(entry.Value); text != "" {
			if desc := strings.TrimSpace(entry.Description); desc != "" {
				result = append(result, desc+":\n"+text)
			} else {
				result = append(result, text)
			}
		}
	}
	return result
}

// encodeToolsAsRaw marshals each AG-UI tool into its JSON representation so
// that it can be forwarded verbatim to downstream consumers (the per-session
// contextual tools store and the NewSessionRequest.Meta payload).
func encodeToolsAsRaw(tools []aguitypes.Tool) []json.RawMessage {
	result := make([]json.RawMessage, 0, len(tools))
	for _, tool := range tools {
		payload, err := json.Marshal(tool)
		if err != nil {
			continue
		}
		result = append(result, json.RawMessage(payload))
	}
	return result
}

// extractText collects textual content from an AG-UI message Content field.
// The field is typed as any because AG-UI allows a plain string, a list of
// typed parts, or an already-decoded structure. Each typed-decode is only
// short-circuited when it actually yields text — a successful decode that
// produces no usable parts (e.g. items lacking the "text" InputContent
// type) falls through so the generic object/array fallbacks can still
// recover the user's input.
func extractText(content any) string {
	if content == nil {
		return ""
	}

	if direct, ok := content.(string); ok {
		return strings.TrimSpace(direct)
	}

	if parts, ok := content.([]aguitypes.InputContent); ok {
		if text := collectInputContentText(parts); text != "" {
			return text
		}
	}

	raw, err := json.Marshal(content)
	if err != nil {
		return ""
	}

	if parts, ok := decodeInputContent(raw); ok {
		if text := collectInputContentText(parts); text != "" {
			return text
		}
	}

	var direct string
	if err := json.Unmarshal(raw, &direct); err == nil {
		return strings.TrimSpace(direct)
	}

	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err == nil {
		if text, ok := obj["text"].(string); ok {
			return strings.TrimSpace(text)
		}
		if parts, ok := obj["parts"].([]any); ok {
			combined := collectTextParts(parts)
			if combined != "" {
				return combined
			}
		}
	}

	var arr []any
	if err := json.Unmarshal(raw, &arr); err == nil {
		return collectTextParts(arr)
	}

	return ""
}

func decodeInputContent(raw []byte) ([]aguitypes.InputContent, bool) {
	var parts []aguitypes.InputContent
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil, false
	}
	return parts, true
}

func collectInputContentText(parts []aguitypes.InputContent) string {
	textParts := make([]string, 0, len(parts))
	for _, part := range parts {
		if part.Type != aguitypes.InputContentTypeText {
			continue
		}
		if trimmed := strings.TrimSpace(part.Text); trimmed != "" {
			textParts = append(textParts, trimmed)
		}
	}
	return strings.Join(textParts, "\n")
}

func collectTextParts(parts []any) string {
	var textParts []string
	for _, part := range parts {
		switch v := part.(type) {
		case string:
			if trimmed := strings.TrimSpace(v); trimmed != "" {
				textParts = append(textParts, trimmed)
			}
		case map[string]any:
			if text, ok := v["text"].(string); ok {
				if trimmed := strings.TrimSpace(text); trimmed != "" {
					textParts = append(textParts, trimmed)
				}
			}
		default:
			continue
		}
	}
	return strings.Join(textParts, "\n")
}

// The helpers below cover the outbound direction: ACP / MCP session-update
// shapes are mapped into the primitives required by AG-UI typed events.
// Streaming lifecycle and ordering concerns live in streaming_client.go.

// valueOrUnknown returns the string form of an ACP ToolCallStatus pointer,
// or "unknown" when the sidecar omits a status. The streaming client uses
// this to decide whether a tool call has reached a terminal state and a
// TOOL_CALL_END frame should be emitted.
func valueOrUnknown(v *acp.ToolCallStatus) string {
	if v == nil {
		return "unknown"
	}
	return string(*v)
}

// toolResultMessageID derives the synthetic "tool" message id that AG-UI's
// TOOL_CALL_RESULT schema requires. Deriving from the toolCallId keeps the
// id stable across retries of the same tool call without per-instance state.
func toolResultMessageID(toolCallID acp.ToolCallId) string {
	return "tool-msg-" + string(toolCallID)
}

// stripUIToolPrefix removes the contextual-tool namespace from a tool name
// so the frontend sees the original name it registered. Non-prefixed
// names (e.g. built-in MCP tools) are returned unchanged.
//
// A name that is exactly UIToolPrefix (e.g. "ui_") would strip to "", which
// AG-UI tool-call events reject and which would break SSE encoding. That
// shape is already rejected as InvalidParams by handleJaegerToolCall on the
// ext_method path, so it should never reach the streaming client; here we
// defend the streaming path independently by falling back to the original
// (still non-empty) name so the run does not terminate over a malformed
// upstream tool name.
func stripUIToolPrefix(name string) string {
	if stripped, ok := strings.CutPrefix(name, UIToolPrefix); ok && stripped != "" {
		return stripped
	}
	return name
}

// marshalToolArgsDelta serializes the sidecar's raw tool arguments to a
// JSON string suitable for TOOL_CALL_ARGS.delta. AG-UI streams args as
// successive deltas concatenated by the frontend; the gateway emits the
// full payload as one delta because the sidecar delivers args atomically.
func marshalToolArgsDelta(raw any) string {
	payload, err := json.Marshal(raw)
	if err != nil {
		return fmt.Sprintf("%v", raw)
	}
	return string(payload)
}

// flattenToolResultContent reduces the sidecar's tool output to the single
// string AG-UI's TOOL_CALL_RESULT.content field expects. The sidecar
// forwards MCP CallToolResult envelopes verbatim — {content:[{type:"text",
// text:"..."}], structuredContent:{...}} — so the text blocks are
// concatenated when present. Anything else is JSON-encoded so the frontend
// always receives a deterministic string instead of a nested object.
func flattenToolResultContent(raw any) string {
	if envelope, ok := raw.(map[string]any); ok {
		if blocks, ok := envelope["content"].([]any); ok {
			var b strings.Builder
			for _, block := range blocks {
				blockMap, ok := block.(map[string]any)
				if !ok {
					continue
				}
				if text, ok := blockMap["text"].(string); ok {
					_, _ = b.WriteString(text)
				}
			}
			if b.Len() > 0 {
				return b.String()
			}
		}
	}
	payload, err := json.Marshal(raw)
	if err != nil {
		return fmt.Sprintf("%v", raw)
	}
	return string(payload)
}
