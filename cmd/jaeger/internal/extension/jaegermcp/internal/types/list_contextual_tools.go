// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package types

// ListContextualToolsInput is the input for the list_contextual_tools MCP tool.
// The sidecar is expected to pass the ACP session ID it received on the
// in-flight PromptRequest so the backend can return the correct per-turn
// snapshot even when multiple chat requests run concurrently.
type ListContextualToolsInput struct {
	SessionID string `json:"session_id" jsonschema:"ACP session id for the in-flight prompt; forward PromptRequest.SessionId verbatim"`
}

// ListContextualToolsOutput is the output for the list_contextual_tools MCP tool.
type ListContextualToolsOutput struct {
	Tools []any `json:"tools" jsonschema:"Frontend-provided AG-UI tools array for the requested session"`
}
