// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"encoding/json"
	"errors"
	"strings"

	aguitypes "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
)

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

func extractText(content any) string {
	if content == nil {
		return ""
	}

	if direct, ok := content.(string); ok {
		return strings.TrimSpace(direct)
	}

	if parts, ok := content.([]aguitypes.InputContent); ok {
		return collectInputContentText(parts)
	}

	raw, err := json.Marshal(content)
	if err != nil {
		return ""
	}
	if len(raw) == 0 {
		return ""
	}

	if parts, ok := decodeInputContent(raw); ok {
		return collectInputContentText(parts)
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
