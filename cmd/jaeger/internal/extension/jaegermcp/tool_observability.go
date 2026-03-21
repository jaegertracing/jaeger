// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegermcp

import (
	"context"
	"errors"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	toolStatusOK              = "ok"
	toolStatusInvalidArgument = "invalid_argument"
	toolStatusNotFound        = "not_found"
	toolStatusError           = "error"
)

var errToolResultMarkedError = errors.New("tool result marked as error")

type toolObservability struct {
	tracer trace.Tracer
}

func newToolObservability(tracerProvider trace.TracerProvider) *toolObservability {
	if tracerProvider == nil {
		tracerProvider = otel.GetTracerProvider()
	}
	return &toolObservability{
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
		ctx, toolSpan := obs.tracer.Start(
			ctx,
			"mcp.tool."+toolName,
			trace.WithAttributes(attribute.String("mcp.tool.name", toolName)),
		)
		defer toolSpan.End()

		result, output, err = handler(ctx, req, input)
		status := normalizeToolStatus(err, result)
		observeToolInSpan(toolSpan, status, spanError(err, result))
		return result, output, err
	}
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
