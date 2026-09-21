// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package mcptools

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"time"
	"unicode/utf8"

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

	metricStatusSuccess = "success"
	metricStatusError   = "error"

	// maxSpanAttrChars bounds gen_ai.tool.call.arguments/result so an oversized
	// tool payload can't trip OTLP attribute-size limits.
	//
	// The SDK does not do this for us: DefaultAttributeValueLengthLimit is -1
	// (unlimited) and Jaeger sets neither SpanLimits nor
	// OTEL_SPAN_ATTRIBUTE_VALUE_LENGTH_LIMIT, so an oversized value is exported
	// in full. Because the batch processor packs many spans into one OTLP
	// request, a single huge value can fail the export of every span batched
	// with it.
	maxSpanAttrChars = 65536
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
				attrs = append(
					attrs,
					otelsemconv.GenAIOperationNameExecuteTool,
					otelsemconv.GenAIToolName(toolName),
				)
				if toolArgs := toolArgumentsFromRequest(method, req); toolArgs != "" {
					attrs = append(attrs, otelsemconv.GenAIToolCallArguments(truncateForSpan(toolArgs, maxSpanAttrChars)))
				}
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
			if callResult, ok := result.(*mcp.CallToolResult); ok {
				if resultText := toolResultText(callResult); resultText != "" {
					span.SetAttributes(otelsemconv.GenAIToolCallResult(truncateForSpan(resultText, maxSpanAttrChars)))
				}
				if callResult.IsError {
					span.SetAttributes(otelsemconv.ErrorType(errorTypeTool))
					if toolErr := callResult.GetError(); toolErr != nil {
						span.RecordError(toolErr)
					}
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

			result, err := next(ctx, method, req)

			status := metricStatusSuccess
			if err != nil {
				status = metricStatusError
			} else if callResult, ok := result.(*mcp.CallToolResult); ok && callResult.IsError {
				status = metricStatusError
			}
			attrs := buildMetricAttributes(method, toolName, status)

			callCounter.Add(ctx, 1, metric.WithAttributes(attrs...))
			durationHistogram.Record(ctx, time.Since(start).Seconds(), metric.WithAttributes(attrs...))

			return result, err
		}
	}, nil
}

func buildMetricAttributes(method, toolName, status string) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, 3)
	attrs = append(attrs, otelsemconv.McpMethodName(method))
	if toolName != "" {
		attrs = append(attrs, otelsemconv.GenAIToolName(toolName))
	}
	attrs = append(attrs, attribute.String("status", status))
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

// toolArgumentsFromRequest returns the raw JSON arguments a tools/call
// request was made with, as received over the wire. Returns "" when the
// method isn't tools/call or carries no arguments.
func toolArgumentsFromRequest(method string, req mcp.Request) string {
	if method != mcpMethodToolsCall || req == nil {
		return ""
	}
	params, ok := req.GetParams().(*mcp.CallToolParamsRaw)
	if !ok || params == nil {
		return ""
	}
	return string(params.Arguments)
}

// toolResultText JSON-encodes a tool call result for the
// gen_ai.tool.call.result span attribute. Falls back to a Go-syntax
// representation if the result cannot be marshaled (defensive; the MCP wire
// types are JSON-safe by construction).
func toolResultText(result *mcp.CallToolResult) string {
	if result == nil {
		return ""
	}
	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Sprintf("%+v", result)
	}
	return string(data)
}

// truncateForSpan caps text at maxChars before it's set as a span attribute
// value. See maxSpanAttrChars for why.
//
// The marker leads rather than trails. maxChars is already long enough that a
// UI will clip or collapse the value, and a trailing marker is the one part a
// reader never sees, whereas a leading one survives every rendering including a
// one-line preview. Opening with "(" also stops a UI that special-cases
// gen_ai.* attributes from parsing the value as JSON and reporting a parse
// error: truncated JSON that still opens with "{" advertises itself as
// parseable while being malformed.
//
// The cut lands on a rune boundary. Tool results carry arbitrary text — service
// names, log messages, span tags — so a byte-boundary cut can split a multi-byte
// rune and leave the value invalid UTF-8. Protobuf string fields must hold valid
// UTF-8 and the Go marshaler enforces it, so such a value fails the OTLP export
// of every span batched with it: precisely the failure this cap exists to
// prevent.
func truncateForSpan(text string, maxChars int) string {
	if len(text) <= maxChars {
		return text
	}
	prefix := fmt.Sprintf("(truncated from %d) ", len(text))
	if maxChars <= len(prefix) {
		// Degenerate cap: keep as much of the marker as fits. It is ASCII, so
		// slicing it cannot produce invalid UTF-8.
		return prefix[:maxChars]
	}
	keep := maxChars - len(prefix)
	// Back off to the start of the rune straddling the cut, if any.
	for keep > 0 && !utf8.RuneStart(text[keep]) {
		keep--
	}
	return prefix + text[:keep]
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
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Func, reflect.Chan, reflect.Interface:
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
