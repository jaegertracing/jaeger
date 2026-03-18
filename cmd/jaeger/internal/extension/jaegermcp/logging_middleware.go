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
func createLoggingMiddleware(logger *zap.Logger) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(
			ctx context.Context,
			method string,
			req mcp.Request,
		) (mcp.Result, error) {
			start := time.Now()
			sessionID := req.GetSession().ID()

			// Log request details.
			logger.Info("MCP request",
				zap.String("session_id", sessionID),
				zap.String("method", method))

			// Call the actual handler.
			result, err := next(ctx, method, req)

			// Log response details.
			duration := time.Since(start)

			if err != nil {
				logger.Error("MCP response",
					zap.String("session_id", sessionID),
					zap.String("method", method),
					zap.Duration("duration", duration),
					zap.Error(err))
			} else {
				logger.Info("MCP response",
					zap.String("session_id", sessionID),
					zap.String("method", method),
					zap.Duration("duration", duration))
			}

			return result, err
		}
	}
}
