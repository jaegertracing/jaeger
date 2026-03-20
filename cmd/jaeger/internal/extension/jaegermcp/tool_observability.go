// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegermcp

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

const (
	toolStatusOK              = "ok"
	toolStatusInvalidArgument = "invalid_argument"
	toolStatusNotFound        = "not_found"
	toolStatusError           = "error"
)

var (
	errToolHandlerPanic      = errors.New("tool handler panicked")
	errToolResultMarkedError = errors.New("tool result marked as error")
)

type toolObservability struct {
	logger *zap.Logger
	tracer trace.Tracer
}

func newToolObservability(logger *zap.Logger, tracerProvider trace.TracerProvider) *toolObservability {
	if logger == nil {
		logger = zap.NewNop()
	}
	if tracerProvider == nil {
		tracerProvider = otel.GetTracerProvider()
	}
	return &toolObservability{
		logger: logger,
		tracer: tracerProvider.Tracer("jaeger.mcp"),
	}
}

func instrumentTool[In, Out any](
	obs *toolObservability,
	toolName string,
	handler mcp.ToolHandlerFor[In, Out],
) mcp.ToolHandlerFor[In, Out] {
	if obs == nil {
		return handler
	}

	return func(ctx context.Context, req *mcp.CallToolRequest, input In) (result *mcp.CallToolResult, output Out, err error) {
		start := time.Now()
		ctx, toolSpan := obs.tracer.Start(
			ctx,
			"mcp.tool."+toolName,
			trace.WithAttributes(attribute.String("mcp.tool.name", toolName)),
		)

		defer func() {
			defer toolSpan.End()

			panicValue := recover()
			if panicValue != nil {
				err = errToolHandlerPanic
			}

			duration := time.Since(start)
			status := normalizeToolStatus(err, result)
			fields := []zap.Field{
				zap.String("tool_name", toolName),
				zap.String("status", status),
				zap.Duration("duration", duration),
			}

			spanErr := err
			if spanErr == nil && result != nil && result.IsError {
				spanErr = result.GetError()
				if spanErr == nil {
					spanErr = errToolResultMarkedError
				}
			}
			observeToolInSpan(toolSpan, status, spanErr)

			if panicValue != nil {
				obs.logger.Error("MCP tool invocation failed", append(fields, zap.Any("panic", panicValue), zap.Error(err))...)
				return
			}

			if err != nil {
				obs.logFailure(status, append(fields, zap.Error(err))...)
				return
			}
			if result != nil && result.IsError {
				if resultErr := result.GetError(); resultErr != nil {
					fields = append(fields, zap.Error(resultErr))
				}
				obs.logFailure(status, fields...)
				return
			}

			if ce := obs.logger.Check(zap.DebugLevel, "MCP tool invocation completed"); ce != nil {
				ce.Write(fields...)
			}
		}()

		return handler(ctx, req, input)
	}
}

func (o *toolObservability) logFailure(status string, fields ...zap.Field) {
	if status == toolStatusInvalidArgument || status == toolStatusNotFound {
		o.logger.Warn("MCP tool invocation failed", fields...)
		return
	}
	o.logger.Error("MCP tool invocation failed", fields...)
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
