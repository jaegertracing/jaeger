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

// MCP method names the UI-dispatch middleware intercepts. The go-sdk keeps its
// own copies of these unexported, so we spell them out here.
const (
	methodListTools = "tools/list"
	methodCallTool  = "tools/call"
)

// uiDispatchMiddleware layers a session's frontend-declared UI tools onto the
// shared telemetry MCP server without registering them on it. It is installed
// as receiving middleware, so it wraps every inbound method and acts on two:
//
//   - tools/list — after the shared server lists the built-in telemetry tools,
//     the calling session's UI tools are appended so the agent can see them.
//     A UI tool whose name matches a telemetry tool shadows it (matching
//     Server.AddTool's replace-by-name semantics), keeping the list free of
//     duplicate names.
//   - tools/call — a call to one of the session's UI tools is dispatched to the
//     browser over its SSE stream (the browser is the executor) and acked;
//     everything else falls through to the telemetry handlers.
//
// The session is resolved from the request context: ServeHTTP stamps the URL
// session id before delegating, and the go-sdk propagates the initialize
// request's context values onto the ServerSession, so the id is recoverable on
// tools/list and tools/call alike. A request with no active session degrades to
// telemetry-only. UI-tool dispatch short-circuits before the telemetry tracing/
// metrics middleware (which this wraps), so browser dispatches don't pollute the
// query-tool instrumentation.
func uiDispatchMiddleware(streams *sessionStreams, logger *zap.Logger) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			switch method {
			case methodListTools:
				res, err := next(ctx, method, req)
				if err != nil {
					return res, err
				}
				lt, ok := res.(*mcp.ListToolsResult)
				if !ok {
					return res, nil
				}
				if sess := streams.get(sessionIDFromContext(ctx)); sess != nil {
					lt.Tools = appendUITools(lt.Tools, sess, logger)
				}
				return lt, nil

			case methodCallTool:
				call, ok := req.(*mcp.CallToolRequest)
				if !ok {
					// Unreachable in practice — the SDK matches the request type to
					// the method — but stay out of the way rather than assume.
					return next(ctx, method, req)
				}
				if call.Params == nil {
					// A tools/call with no params is malformed. Return a tool error
					// rather than forwarding it: the downstream handler dereferences
					// Params.Name unconditionally and would panic on nil.
					return uiToolErrorResult("missing tool call parameters"), nil
				}
				sess := streams.get(sessionIDFromContext(ctx))
				if sess == nil || !sessionDeclaredUITool(sess, call.Params.Name) {
					return next(ctx, method, req)
				}
				return dispatchUIToolCall(sess.stream, call.Params.Name, call.Params.Arguments), nil

			default:
				return next(ctx, method, req)
			}
		}
	}
}

// appendUITools returns the telemetry tool list with the session's UI tools
// added. Any telemetry tool shadowed by a same-named UI tool is dropped so the
// result carries a single entry per name (UI wins, mirroring AddTool's
// replace-by-name behaviour). The input slice is not mutated. Malformed UI tools
// are skipped and logged.
func appendUITools(telemetryTools []*mcp.Tool, sess *session, logger *zap.Logger) []*mcp.Tool {
	uiTools := uiToolDescriptors(sess, logger)
	if len(uiTools) == 0 {
		return telemetryTools
	}
	shadowed := make(map[string]struct{}, len(uiTools))
	for _, t := range uiTools {
		shadowed[t.Name] = struct{}{}
	}
	merged := make([]*mcp.Tool, 0, len(telemetryTools)+len(uiTools))
	for _, t := range telemetryTools {
		if _, clash := shadowed[t.Name]; !clash {
			merged = append(merged, t)
		}
	}
	return append(merged, uiTools...)
}

// uiToolDef is a frontend UI tool parsed into the fields the endpoint needs to
// advertise it (name, description) and route calls to it (name), with an
// MCP-acceptable input schema.
type uiToolDef struct {
	name        string
	description string
	schema      map[string]any
}

// uiToolDescriptors parses the session's declared UI tools into MCP tool
// descriptors for advertisement in tools/list, skipping malformed entries
// (frontend input is untrusted) and collapsing repeated names to their first
// occurrence so the advertised list has no duplicates. The InputSchema is
// normalized to a valid JSON-object schema so a frontend typo can't make the
// agent reject the tool. These descriptors are advertised only, never registered
// on the server — dispatch is handled by the middleware — so they bypass
// Server.AddTool's schema validation by design.
func uiToolDescriptors(sess *session, logger *zap.Logger) []*mcp.Tool {
	descriptors := make([]*mcp.Tool, 0, len(sess.uiTools))
	seen := make(map[string]struct{}, len(sess.uiTools))
	for _, raw := range sess.uiTools {
		def, ok := parseUITool(raw)
		if !ok {
			logger.Warn("skipping malformed UI tool", zap.ByteString("tool", raw))
			continue
		}
		if _, dup := seen[def.name]; dup {
			continue // a frontend that declares the same tool twice gets one entry
		}
		seen[def.name] = struct{}{}
		descriptors = append(descriptors, &mcp.Tool{
			Name:        def.name,
			Description: def.description,
			InputSchema: def.schema,
		})
	}
	return descriptors
}

// sessionDeclaredUITool reports whether toolName is one of the session's
// frontend-declared UI tools. Malformed or unnamed entries never match.
func sessionDeclaredUITool(sess *session, toolName string) bool {
	for _, raw := range sess.uiTools {
		if def, ok := parseUITool(raw); ok && def.name == toolName {
			return true
		}
	}
	return false
}

// parseUITool extracts a uiToolDef from a frontend tool definition. ok is false
// when the JSON is malformed or the tool carries no name.
func parseUITool(raw json.RawMessage) (uiToolDef, bool) {
	var tool map[string]any
	if err := json.Unmarshal(raw, &tool); err != nil {
		return uiToolDef{}, false
	}
	name, _ := tool["name"].(string)
	if name == "" {
		return uiToolDef{}, false
	}
	description, _ := tool["description"].(string)
	return uiToolDef{
		name:        name,
		description: description,
		schema:      normalizeUIToolSchema(tool["parameters"]),
	}, true
}

// dispatchUIToolCall fires the UI tool's TOOL_CALL_* lifecycle onto the browser
// SSE stream and returns a synthetic ack so the agent's tool-call loop can
// progress. The browser is the real executor, so this is fire-and-forget at the
// LLM layer. A nil stream (session ended mid-request) or malformed arguments
// return an MCP error result rather than failing the call at the transport.
func dispatchUIToolCall(stream *streamingClient, toolName string, rawArgs json.RawMessage) *mcp.CallToolResult {
	if stream == nil {
		return uiToolErrorResult(fmt.Sprintf("session stream closed for tool %q", toolName))
	}
	var args any
	if len(rawArgs) > 0 {
		if err := json.Unmarshal(rawArgs, &args); err != nil {
			return uiToolErrorResult(fmt.Sprintf("invalid JSON arguments for tool %q: %v", toolName, err))
		}
	}
	stream.EmitContextualToolCall(newUIToolCallID(toolName), toolName, args)
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("ui tool %q dispatched to the browser", toolName)}},
	}
}

// uiToolErrorResult builds an IsError CallToolResult carrying msg, so a bad
// dispatch is reported to the agent as a tool error instead of a transport
// failure.
func uiToolErrorResult(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: msg}},
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

// uiToolCallIDSeq is a process-wide monotonic counter appended to generated
// tool-call ids so two dispatches within the same nanosecond don't collide.
var uiToolCallIDSeq atomic.Uint64

// newUIToolCallID produces a stable per-process unique id for a TOOL_CALL_*
// event group. The browser treats it as opaque; name-first keeps logs readable
// and nanos+counter guarantees uniqueness.
func newUIToolCallID(name string) string {
	return fmt.Sprintf("%s-%d-%d", name, time.Now().UnixNano(), uiToolCallIDSeq.Add(1))
}
