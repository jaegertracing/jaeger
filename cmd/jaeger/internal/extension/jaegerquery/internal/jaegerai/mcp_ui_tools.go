// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/telemetry/otelsemconv"
)

// MCP method names the UI-dispatch middleware intercepts. The go-sdk keeps its
// own copies of these unexported, so we spell them out here.
const (
	methodListTools = "tools/list"
	methodCallTool  = "tools/call"
)

// uiToolsMiddleware layers a turn's frontend-declared UI tools onto the
// shared telemetry MCP server without registering them on it. It is installed
// as receiving middleware, so it wraps every inbound method and acts on two:
//
//   - tools/list — after the shared server lists the built-in telemetry tools,
//     the calling turn's UI tools are appended so the agent can see them.
//     UI tools are prefixed with "ui_" for namespace isolation.
//   - tools/call — a call to one of the turn's UI tools is dispatched to the
//     browser over its SSE stream via dispatchUITool; calls to telemetry tools
//     are forwarded upstream via forwardToUpstream.
func uiToolsMiddleware(turns *turnRegistry, logger *zap.Logger, optTracer ...oteltrace.Tracer) mcp.Middleware {
	var tracer oteltrace.Tracer
	if len(optTracer) > 0 && optTracer[0] != nil {
		tracer = optTracer[0]
	} else {
		tracer = otel.GetTracerProvider().Tracer("jaeger.ai.mcp")
	}

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
				if turn := turns.get(mcpRouteIDFromContext(ctx)); turn != nil {
					lt.Tools = appendUITools(lt.Tools, turn, logger)
				}
				return lt, nil

			case methodCallTool:
				call, ok := req.(*mcp.CallToolRequest)
				if !ok {
					return next(ctx, method, req)
				}
				if call.Params == nil {
					return uiToolErrorResult("missing tool call parameters"), nil
				}
				turn := turns.get(mcpRouteIDFromContext(ctx))
				if turn != nil && turnDeclaredUITool(turn, call.Params.Name) {
					return dispatchUITool(ctx, turn, call.Params.Name, call.Params.Arguments, tracer, logger), nil
				}
				return forwardToUpstream(ctx, turn, call.Params.Name, call.Params.Arguments, func(callCtx context.Context) (*mcp.CallToolResult, error) {
					res, err := next(callCtx, method, req)
					if err != nil {
						return nil, err
					}
					if cr, ok := res.(*mcp.CallToolResult); ok {
						return cr, nil
					}
					return nil, fmt.Errorf("unexpected response type: %T", res)
				}, tracer, logger)

			default:
				return next(ctx, method, req)
			}
		}
	}
}

// appendUITools returns the telemetry tool list with the turn's UI tools
// added. Any telemetry tool shadowed by a same-named UI tool is dropped so the
// result carries a single entry per name (UI wins, mirroring AddTool's
// replace-by-name behaviour). The input slice is not mutated. Malformed UI tools
// are skipped and logged.
func appendUITools(telemetryTools []*mcp.Tool, turn *turnState, logger *zap.Logger) []*mcp.Tool {
	uiTools := uiToolDescriptors(turn, logger)
	if len(uiTools) == 0 {
		return telemetryTools
	}
	shadowed := make(map[string]struct{}, len(uiTools))
	for _, t := range uiTools {
		shadowed[t.Name] = struct{}{}
		shadowed[strings.TrimPrefix(t.Name, UIToolPrefix)] = struct{}{}
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

// uiToolDescriptors parses the turn's declared UI tools into MCP tool
// descriptors for advertisement in tools/list, prefixing names with "ui_"
// to avoid collisions with telemetry tools.
func uiToolDescriptors(turn *turnState, logger *zap.Logger) []*mcp.Tool {
	descriptors := make([]*mcp.Tool, 0, len(turn.uiTools))
	seen := make(map[string]struct{}, len(turn.uiTools))
	for _, raw := range turn.uiTools {
		def, ok := parseUITool(raw)
		if !ok {
			logger.Warn("skipping malformed UI tool", zap.ByteString("tool", raw))
			continue
		}
		if _, dup := seen[def.name]; dup {
			continue // a frontend that declares the same tool twice gets one entry
		}
		seen[def.name] = struct{}{}
		name := def.name
		if !strings.HasPrefix(name, UIToolPrefix) {
			name = UIToolPrefix + name
		}
		descriptors = append(descriptors, &mcp.Tool{
			Name:        name,
			Description: def.description,
			InputSchema: def.schema,
		})
	}
	return descriptors
}

// turnDeclaredUITool reports whether toolName is one of the turn's
// frontend-declared UI tools, matching either the prefixed or unprefixed name.
func turnDeclaredUITool(turn *turnState, toolName string) bool {
	stripped := strings.TrimPrefix(toolName, UIToolPrefix)
	for _, raw := range turn.uiTools {
		if def, ok := parseUITool(raw); ok {
			if def.name == toolName || def.name == stripped || UIToolPrefix+def.name == toolName {
				return true
			}
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

// dispatchUITool wraps execution of a UI tool with OpenTelemetry tracing, emits
// the call to the browser stream, and returns a synthetic acknowledgment.
func dispatchUITool(ctx context.Context, turn *turnState, toolName string, rawArgs json.RawMessage, tracer oteltrace.Tracer, logger *zap.Logger) *mcp.CallToolResult {
	if logger != nil {
		logger.Debug("dispatching UI tool", zap.String("tool", toolName))
	}
	if tracer == nil {
		tracer = otel.GetTracerProvider().Tracer("jaeger.ai.mcp")
	}
	spanName := "dispatchUITool " + toolName
	attrs := []attribute.KeyValue{
		otelsemconv.GenAIOperationNameExecuteTool,
		otelsemconv.GenAIToolName(toolName),
		otelsemconv.McpMethodName(methodCallTool),
	}
	if len(rawArgs) > 0 {
		attrs = append(attrs, otelsemconv.GenAIToolCallArguments(string(rawArgs)))
	}
	_, span := tracer.Start(ctx, spanName,
		oteltrace.WithSpanKind(oteltrace.SpanKindInternal),
		oteltrace.WithAttributes(attrs...),
	)
	defer span.End()

	strippedName := strings.TrimPrefix(toolName, UIToolPrefix)
	var stream *streamingClient
	if turn != nil {
		stream = turn.stream
	}
	res := emitUIToolCall(stream, strippedName, rawArgs)
	if res.IsError {
		span.SetAttributes(otelsemconv.ErrorType("tool_error"))
		if len(res.Content) > 0 {
			if tc, ok := res.Content[0].(*mcp.TextContent); ok {
				span.SetStatus(codes.Error, tc.Text)
			}
		}
	} else if len(res.Content) > 0 {
		if tc, ok := res.Content[0].(*mcp.TextContent); ok {
			span.SetAttributes(otelsemconv.GenAIToolCallResult(tc.Text))
		}
	}
	return res
}

// forwardToUpstream forwards a telemetry tool call to the upstream executor,
// wrapping execution in an OpenTelemetry span and emitting SSE tool-call events
// to the browser stream if enabled.
func forwardToUpstream(ctx context.Context, turn *turnState, toolName string, rawArgs json.RawMessage, execute func(context.Context) (*mcp.CallToolResult, error), tracer oteltrace.Tracer, logger *zap.Logger) (*mcp.CallToolResult, error) {
	if logger != nil {
		logger.Debug("forwarding telemetry tool upstream", zap.String("tool", toolName))
	}
	if tracer == nil {
		tracer = otel.GetTracerProvider().Tracer("jaeger.ai.mcp")
	}
	spanName := "forwardToUpstream " + toolName
	attrs := []attribute.KeyValue{
		otelsemconv.GenAIOperationNameExecuteTool,
		otelsemconv.GenAIToolName(toolName),
		otelsemconv.McpMethodName(methodCallTool),
	}
	if len(rawArgs) > 0 {
		attrs = append(attrs, otelsemconv.GenAIToolCallArguments(string(rawArgs)))
	}
	ctx, span := tracer.Start(ctx, spanName,
		oteltrace.WithSpanKind(oteltrace.SpanKindInternal),
		oteltrace.WithAttributes(attrs...),
	)
	defer span.End()

	// Handle SSE emission during forwardToUpstream
	if turn != nil && turn.stream != nil && !turn.disableStandaloneSSE {
		var args any
		if len(rawArgs) > 0 {
			_ = json.Unmarshal(rawArgs, &args)
		}
		turn.stream.EmitContextualToolCall(newUIToolCallID(toolName), toolName, args)
	}

	res, err := execute(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	if res != nil {
		if res.IsError {
			span.SetAttributes(otelsemconv.ErrorType("tool_error"))
			if len(res.Content) > 0 {
				if tc, ok := res.Content[0].(*mcp.TextContent); ok {
					span.SetStatus(codes.Error, tc.Text)
				}
			}
		} else if len(res.Content) > 0 {
			if tc, ok := res.Content[0].(*mcp.TextContent); ok {
				span.SetAttributes(otelsemconv.GenAIToolCallResult(tc.Text))
			}
		}
	}
	return res, nil
}

// emitUIToolCall fires the UI tool's TOOL_CALL_* lifecycle onto the browser
// SSE stream and returns a synthetic ack so the agent's tool-call loop can
// progress. The browser is the real executor, so this is fire-and-forget at the
// LLM layer. A nil stream (turn ended mid-request) or malformed arguments
// return an MCP error result rather than failing the call at the transport.
func emitUIToolCall(stream *streamingClient, toolName string, rawArgs json.RawMessage) *mcp.CallToolResult {
	if stream == nil {
		return uiToolErrorResult(fmt.Sprintf("browser stream closed for tool %q", toolName))
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
