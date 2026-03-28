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
	"go.uber.org/zap"
)

const (
	toolStatusOK       = "ok"
	toolStatusError    = "error"
	mcpMethodToolsCall = "tools/call"
)

// createLoggingMiddleware creates an MCP middleware that logs request/response details.
func createLoggingMiddleware(logger *zap.Logger) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(
			ctx context.Context,
			method string,
			req mcp.Request,
		) (mcp.Result, error) {
			sessionID := sessionIDFromRequest(req)
			toolName := toolNameFromRequest(method, req)

			requestFields := []zap.Field{
				zap.String("session_id", sessionID),
				zap.String("method", method),
			}
			if toolName != "" {
				requestFields = append(requestFields, zap.String("tool_name", toolName))
			}
			logger.Info("MCP request", requestFields...)

			result, err := next(ctx, method, req)

			responseFields := []zap.Field{
				zap.String("session_id", sessionID),
				zap.String("method", method),
			}
			if toolName != "" {
				responseFields = append(responseFields, zap.String("tool_name", toolName))
			}

			callResult, _ := result.(*mcp.CallToolResult)
			status := ""
			if err != nil || callResult != nil {
				status = normalizeToolStatus(err, callResult)
				responseFields = append(responseFields, zap.String("status", status))
			}
			if err != nil {
				responseFields = append(responseFields, zap.Error(err))
				logger.Error("MCP response", responseFields...)
				return result, err
			}
			if callResult != nil && callResult.IsError {
				if toolErr := spanError(err, callResult); toolErr != nil {
					responseFields = append(responseFields, zap.Error(toolErr))
				}
			}
			logger.Info("MCP response", responseFields...)

			return result, err
		}
	}
}

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
			if err != nil || (callResult != nil && callResult.IsError) {
				span.SetAttributes(semconv.ErrorTypeKey.String(toolStatusError))
				if toolErr := spanError(err, callResult); toolErr != nil {
					span.RecordError(toolErr)
					span.SetStatus(codes.Error, toolErr.Error())
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

func normalizeToolStatus(err error, result *mcp.CallToolResult) string {
	if err == nil && (result == nil || !result.IsError) {
		return toolStatusOK
	}
	return toolStatusError
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
