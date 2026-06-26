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

// validateContextualToolNames rejects requests carrying tools with
// empty or whitespace-only names. Such entries would land in
// ContextualToolsStore as unusable rows and would produce the wire
// shape "ui_" / "ui_   " when the proxy prefixes them, breaking the
// MCP tools/list payload. Returning a 400 from the caller (handler.go)
// surfaces frontend bugs immediately instead of letting them fail
// mid-turn.
//
// The reported error names the first offending tool index so the
// frontend developer can locate the broken declaration; it does not
// reveal any user content from the rest of the request.
func validateContextualToolNames(tools []aguitypes.Tool) error {
	for i, tool := range tools {
		if strings.TrimSpace(tool.Name) == "" {
			return fmt.Errorf("tools[%d].name is empty or whitespace", i)
		}
	}
	return nil
}

// encodeToolsAsRaw marshals each AG-UI tool into its JSON representation
// so the per-session ContextualToolsStore can hand a fresh tree to each
// reader (the MCP proxy unmarshals them on every tools/list / tools/call).
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
	for i := range parts {
		if parts[i].Type != aguitypes.InputContentTypeText {
			continue
		}
		if trimmed := strings.TrimSpace(parts[i].Text); trimmed != "" {
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
// text:"..."}, ...], structuredContent:{...}} — so each text block is
// collected and the result is joined with "\n" to keep block boundaries
// readable (concatenating with no delimiter would mash distinct paragraphs
// like "Found 3 services" + "Top latency: 1.2s" into one run-on string).
// Anything else is JSON-encoded so the frontend always receives a
// deterministic string instead of a nested object.
func flattenToolResultContent(raw any) string {
	if envelope, ok := raw.(map[string]any); ok {
		if blocks, ok := envelope["content"].([]any); ok {
			texts := make([]string, 0, len(blocks))
			for _, block := range blocks {
				blockMap, ok := block.(map[string]any)
				if !ok {
					continue
				}
				if text, ok := blockMap["text"].(string); ok {
					texts = append(texts, text)
				}
			}
			if len(texts) > 0 {
				return strings.Join(texts, "\n")
			}
		}
	}
	payload, err := json.Marshal(raw)
	if err != nil {
		return fmt.Sprintf("%v", raw)
	}
	return string(payload)
}
