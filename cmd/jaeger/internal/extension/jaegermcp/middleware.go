// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegermcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	mcpMethodToolsCall = "tools/call"
	errorTypeTool      = "tool_error"
)

// createTracingMiddleware creates an MCP middleware that emits tool-level spans.
func createTracingMiddleware(tracerProvider trace.TracerProvider) mcp.Middleware {
	tracer := tracerProvider.Tracer("jaeger.mcp")

	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			toolName := toolNameFromRequest(method, req)
			sessionID := sessionIDFromRequest(req)
			spanName := method
			attrs := []attribute.KeyValue{
				semconv.McpMethodNameKey.String(method),
			}
			if toolName != "" {
				spanName = method + " " + toolName
				attrs = append(attrs,
					semconv.GenAIOperationNameExecuteTool,
					semconv.GenAIToolName(toolName),
				)
			}
			if sessionID != "" {
				attrs = append(attrs, semconv.McpSessionID(sessionID))
			}

			ctx, span := tracer.Start(
				ctx,
				spanName,
				trace.WithSpanKind(trace.SpanKindServer),
				trace.WithAttributes(attrs...),
			)
			defer span.End()

			result, err := next(ctx, method, req)

			callResult, _ := result.(*mcp.CallToolResult)
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
				return result, err
			}
			if callResult != nil && callResult.IsError {
				span.SetAttributes(semconv.ErrorTypeKey.String(errorTypeTool))
				if toolErr := spanError(nil, callResult); toolErr != nil {
					span.RecordError(toolErr)
				}
			}

			return result, err
		}
	}
}

func toolNameFromRequest(method string, req mcp.Request) string {
	if method != mcpMethodToolsCall || req == nil {
		return ""
	}
	params, ok := req.GetParams().(*mcp.CallToolParamsRaw)
	if !ok || params == nil {
		return ""
	}
	return params.Name
}

func spanError(err error, result *mcp.CallToolResult) error {
	if err != nil {
		return err
	}
	if result == nil || !result.IsError {
		return nil
	}
	return result.GetError()
}

func sessionIDFromRequest(req mcp.Request) string {
	if req == nil {
		return ""
	}
	session := req.GetSession()
	if session == nil {
		return ""
	}
	if s, ok := session.(*mcp.ServerSession); ok {
		if s == nil {
			return ""
		}
	}
	if s, ok := session.(*mcp.ClientSession); ok {
		if s == nil {
			return ""
		}
	}
	return session.ID()
}
