// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegermcp

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/jaegertracing/jaeger/internal/telemetry/otelsemconv"
)

const (
	mcpMethodToolsCall = "tools/call"
	errorTypeTool      = "tool_error"

	traceContextMetaTraceParent = "traceparent"
	traceContextMetaTraceState  = "tracestate"
	traceContextMetaBaggage     = "baggage"
)

var requestMetaPropagator = propagation.NewCompositeTextMapPropagator(
	propagation.TraceContext{},
	propagation.Baggage{},
)

// createTracingMiddleware creates an MCP middleware that emits tool-level spans.
func createTracingMiddleware(tracerProvider trace.TracerProvider) mcp.Middleware {
	tracer := tracerProvider.Tracer("jaeger.mcp")

	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			ctx = contextWithRequestMetaTraceContext(ctx, req)

			toolName := toolNameFromRequest(method, req)
			sessionID := sessionIDFromRequest(req)
			spanName := method
			attrs := []attribute.KeyValue{}
			if toolName != "" {
				spanName = method + " " + toolName
				attrs = append(attrs,
					otelsemconv.GenAIOperationNameExecuteTool,
					otelsemconv.GenAIToolName(toolName),
				)
			} else {
				attrs = append(attrs, otelsemconv.McpMethodName(method))
			}
			if sessionID != "" {
				attrs = append(attrs, otelsemconv.McpSessionID(sessionID))
			}

			ctx, span := tracer.Start(
				ctx,
				spanName,
				trace.WithSpanKind(trace.SpanKindInternal),
				trace.WithAttributes(attrs...),
			)
			defer span.End()

			result, err := next(ctx, method, req)
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
				return result, err
			}
			if callResult, ok := result.(*mcp.CallToolResult); ok && callResult.IsError {
				span.SetAttributes(otelsemconv.ErrorType(errorTypeTool))
				if toolErr := callResult.GetError(); toolErr != nil {
					span.RecordError(toolErr)
				}
			}

			return result, err
		}
	}
}

// createMetricsMiddleware creates an MCP middleware that records per-method and per-tool invocation metrics.
func createMetricsMiddleware(meterProvider metric.MeterProvider) (mcp.Middleware, error) {
	meter := meterProvider.Meter("jaeger.mcp")
	callCounter, err := meter.Int64Counter(
		"jaeger.mcp.tool.calls",
		metric.WithDescription("Number of MCP method/tool invocations"),
		metric.WithUnit("{call}"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create tool calls counter: %w", err)
	}
	durationHistogram, err := meter.Float64Histogram(
		"jaeger.mcp.tool.duration",
		metric.WithDescription("Duration of MCP method/tool invocations"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create tool duration histogram: %w", err)
	}

	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			start := time.Now()
			toolName := toolNameFromRequest(method, req)
			attrs := buildMetricAttributes(method, toolName)

			result, err := next(ctx, method, req)

			status := metricStatusSuccess
			if err != nil {
				status = metricStatusError
			} else if callResult, ok := result.(*mcp.CallToolResult); ok && callResult.IsError {
				status = metricStatusError
			}
			attrs = append(attrs, statusAttr(status))

			callCounter.Add(ctx, 1, metric.WithAttributes(attrs...))
			durationHistogram.Record(ctx, time.Since(start).Seconds(), metric.WithAttributes(attrs...))

			return result, err
		}
	}, nil
}

const (
	metricStatusSuccess = "success"
	metricStatusError   = "error"
)

func statusAttr(status string) attribute.KeyValue {
	return attribute.String("status", status)
}

func buildMetricAttributes(method, toolName string) []attribute.KeyValue {
	attrs := []attribute.KeyValue{otelsemconv.McpMethodName(method)}
	if toolName != "" {
		attrs = append(attrs, otelsemconv.GenAIToolName(toolName))
	}
	return attrs
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

func sessionIDFromRequest(req mcp.Request) string {
	if req == nil {
		return ""
	}
	session := req.GetSession()
	if isNil(session) {
		return ""
	}
	return session.ID()
}

func contextWithRequestMetaTraceContext(ctx context.Context, req mcp.Request) context.Context {
	if req == nil {
		return ctx
	}

	params := req.GetParams()
	if isNil(params) {
		return ctx
	}

	return requestMetaPropagator.Extract(ctx, &requestMetaCarrier{meta: params.GetMeta()})
}

func isNil(value any) bool {
	if value == nil {
		return true
	}
	reflectValue := reflect.ValueOf(value)
	switch reflectValue.Kind() {
	case reflect.Ptr, reflect.Map, reflect.Slice, reflect.Func, reflect.Chan, reflect.Interface:
		return reflectValue.IsNil()
	default:
		return false
	}
}

type requestMetaCarrier struct {
	meta mcp.Meta
}

func (carrier *requestMetaCarrier) Get(key string) string {
	value, _ := carrier.meta[key].(string)
	return value
}

func (carrier *requestMetaCarrier) Set(key, value string) {
	if carrier.meta == nil {
		carrier.meta = mcp.Meta{}
	}
	carrier.meta[key] = value
}

func (carrier *requestMetaCarrier) Keys() []string {
	keys := make([]string, 0, len(carrier.meta))
	for key := range carrier.meta {
		keys = append(keys, key)
	}
	return keys
}
