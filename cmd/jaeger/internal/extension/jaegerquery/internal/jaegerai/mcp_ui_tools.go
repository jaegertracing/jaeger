// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"
)

// addUITools registers the session's frontend-declared UI tools on the MCP
// server. Each handler dispatches the call to the browser over the session's
// SSE stream — the browser is the real executor, so dispatch is fire-and-forget
// at the LLM layer. A single malformed tool is skipped and logged rather than
// failing the whole server.
func addUITools(srv *mcp.Server, sess *session, logger *zap.Logger) {
	for _, raw := range sess.uiTools {
		var tool map[string]any
		if err := json.Unmarshal(raw, &tool); err != nil {
			continue
		}
		name, _ := tool["name"].(string)
		if name == "" {
			continue
		}
		description, _ := tool["description"].(string)
		if err := safeAddTool(srv, &mcp.Tool{
			Name:        name,
			Description: description,
			InputSchema: normalizeUIToolSchema(tool["parameters"]),
		}, uiToolHandler(sess.stream, name)); err != nil {
			logger.Warn("skipping malformed UI tool", zap.String("tool", name), zap.Error(err))
		}
	}
}

// uiToolHandler returns an MCP ToolHandler that fires the TOOL_CALL_* lifecycle
// for a UI tool onto the session's browser SSE stream and returns a synthetic
// ack so the agent's tool-call loop can progress.
func uiToolHandler(stream *streamingClient, toolName string) mcp.ToolHandler {
	return func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args any
		if len(req.Params.Arguments) > 0 {
			if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
				return &mcp.CallToolResult{
					IsError: true,
					Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("invalid JSON arguments for tool %q: %v", toolName, err)}},
				}, nil
			}
		}
		stream.EmitContextualToolCall(newUIToolCallID(toolName), toolName, args)
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("ui tool %q dispatched to the browser", toolName)}},
		}, nil
	}
}

// normalizeUIToolSchema coerces whatever the frontend put in `parameters` into a
// JSON-object schema MCP will accept, degrading anything non-conforming to
// {"type":"object"} so a schema typo can't reject the tool outright.
func normalizeUIToolSchema(raw any) map[string]any {
	if m, ok := raw.(map[string]any); ok {
		if typ, _ := m["type"].(string); typ == "object" {
			return m
		}
	}
	return map[string]any{"type": "object"}
}

// safeAddTool wraps Server.AddTool's panic-on-invalid-schema behaviour in a
// recover so one bad frontend tool can't bring down the per-session server —
// the frontend is untrusted input, so a bad schema is a skip-this-tool decision,
// not a crash.
func safeAddTool(s *mcp.Server, t *mcp.Tool, h mcp.ToolHandler) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	s.AddTool(t, h)
	return nil
}

// uiToolCallIDSeq is a process-wide monotonic counter appended to generated
// tool-call ids so two dispatches within the same nanosecond don't collide.
var uiToolCallIDSeq atomic.Uint64

// newUIToolCallID produces a stable per-process unique id for a TOOL_CALL_*
// event group. The browser treats it as opaque; name-first keeps logs readable
// and nanos+counter guarantees uniqueness.
func newUIToolCallID(name string) string {
	return fmt.Sprintf("%s-%d-%d", name, time.Now().UnixNano(), uiToolCallIDSeq.Add(1))
}
