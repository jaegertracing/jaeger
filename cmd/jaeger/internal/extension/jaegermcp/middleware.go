// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegermcp

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"
)

const (
	toolStatusOK              = "ok"
	toolStatusInvalidArgument = "invalid_argument"
	toolStatusNotFound        = "not_found"
	toolStatusError           = "error"
	mcpMethodToolsCall        = "tools/call"
)

var errToolResultMarkedError = errors.New("tool result marked as error")

// createLoggingMiddleware creates an MCP middleware that handles request logging
// and tool-level tracing observability.
func createLoggingMiddleware(logger *zap.Logger, tracerProvider trace.TracerProvider) mcp.Middleware {
	if logger == nil {
		logger = zap.NewNop()
	}
	tracer := newToolTracer(tracerProvider)

	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(
			ctx context.Context,
			method string,
			req mcp.Request,
		) (mcp.Result, error) {
			start := time.Now()
			sessionID := sessionIDFromRequest(req)
			toolName := toolNameFromRequest(method, req)

			var toolSpan trace.Span
			if toolName != "" {
				ctx, toolSpan = tracer.Start(
					ctx,
					"mcp.tool."+toolName,
					trace.WithAttributes(attribute.String("mcp.tool.name", toolName)),
				)
				defer toolSpan.End()
			}

			requestFields := []zap.Field{
				zap.String("session_id", sessionID),
				zap.String("method", method),
			}
			if toolName != "" {
				requestFields = append(requestFields, zap.String("tool_name", toolName))
			}
			logger.Info("MCP request", requestFields...)

			result, err := next(ctx, method, req)

			duration := time.Since(start)
			responseFields := []zap.Field{
				zap.String("session_id", sessionID),
				zap.String("method", method),
				zap.Duration("duration", duration),
			}
			if toolName != "" {
				responseFields = append(responseFields, zap.String("tool_name", toolName))
			}

			status := ""
			toolErr := error(nil)
			if toolName != "" {
				callResult := callToolResult(result)
				status = normalizeToolStatus(err, callResult)
				toolErr = spanError(err, callResult)
				observeToolInSpan(toolSpan, status, toolErr)
				responseFields = append(responseFields, zap.String("status", status))
			}

			switch {
			case err != nil:
				responseFields = append(responseFields, zap.Error(err))
				logFailure(logger, status, responseFields...)
			case status != "" && status != toolStatusOK:
				if toolErr != nil {
					responseFields = append(responseFields, zap.Error(toolErr))
				}
				logFailure(logger, status, responseFields...)
			default:
				logger.Info("MCP response", responseFields...)
			}

			return result, err
		}
	}
}

func newToolTracer(tracerProvider trace.TracerProvider) trace.Tracer {
	if tracerProvider == nil {
		tracerProvider = noop.NewTracerProvider()
	}
	return tracerProvider.Tracer("jaeger.mcp")
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

func callToolResult(result mcp.Result) *mcp.CallToolResult {
	r, ok := result.(*mcp.CallToolResult)
	if !ok {
		return nil
	}
	return r
}

func spanError(err error, result *mcp.CallToolResult) error {
	if err != nil {
		return err
	}
	if result == nil || !result.IsError {
		return nil
	}
	if resultErr := result.GetError(); resultErr != nil {
		return resultErr
	}
	return errToolResultMarkedError
}

func observeToolInSpan(span trace.Span, status string, err error) {
	if !span.IsRecording() {
		return
	}

	span.SetAttributes(attribute.String("mcp.status", status))

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
}

func normalizeToolStatus(err error, result *mcp.CallToolResult) string {
	if err == nil && (result == nil || !result.IsError) {
		return toolStatusOK
	}

	message := ""
	switch {
	case err != nil:
		message = strings.ToLower(err.Error())
	case result != nil && result.GetError() != nil:
		message = strings.ToLower(result.GetError().Error())
	default:
		// Keep empty message when the SDK did not provide a concrete error.
	}

	if strings.Contains(message, "not found") {
		return toolStatusNotFound
	}
	if strings.Contains(message, "invalid") ||
		strings.Contains(message, "required") ||
		strings.Contains(message, "must") ||
		strings.Contains(message, "malformed") ||
		strings.Contains(message, "unsupported") ||
		strings.Contains(message, "empty") {
		return toolStatusInvalidArgument
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

func logFailure(logger *zap.Logger, status string, fields ...zap.Field) {
	if status == toolStatusInvalidArgument || status == toolStatusNotFound {
		logger.Warn("MCP response", fields...)
		return
	}
	logger.Error("MCP response", fields...)
}
