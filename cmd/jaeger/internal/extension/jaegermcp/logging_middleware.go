// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegermcp

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"
)

// createLoggingMiddleware creates an MCP middleware that logs method calls.
// For tools/call requests it also logs the tool name.
func createLoggingMiddleware(logger *zap.Logger) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(
			ctx context.Context,
			method string,
			req mcp.Request,
		) (mcp.Result, error) {
			start := time.Now()
			sessionID := req.GetSession().ID()

			fields := []zap.Field{
				zap.String("session_id", sessionID),
				zap.String("method", method),
			}
			if toolName := extractToolName(method, req); toolName != "" {
				fields = append(fields, zap.String("tool", toolName))
			}

			logger.Info("MCP request", fields...)

			// Call the actual handler.
			result, err := next(ctx, method, req)

			// Log response details.
			fields = append(fields, zap.Duration("duration", time.Since(start)))
			if err != nil {
				fields = append(fields, zap.Error(err))
				logger.Error("MCP response", fields...)
			} else {
				logger.Info("MCP response", fields...)
			}

			return result, err
		}
	}
}

// extractToolName returns the tool name from a tools/call request, or empty string otherwise.
func extractToolName(method string, req mcp.Request) string {
	if method != "tools/call" {
		return ""
	}
	if params, ok := req.GetParams().(*mcp.CallToolParamsRaw); ok {
		return params.Name
	}
	return ""
}
