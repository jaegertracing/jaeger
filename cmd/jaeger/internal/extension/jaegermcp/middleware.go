// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegermcp

import (
	"context"
	"reflect"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/codes"
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

var traceContextMetaKeys = [...]string{
	traceContextMetaTraceParent,
	traceContextMetaTraceState,
	traceContextMetaBaggage,
}

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
	if isNilSession(session) {
		return ""
	}
	return session.ID()
}

func isNilSession(session mcp.Session) bool {
	return isNil(session)
}

func contextWithRequestMetaTraceContext(ctx context.Context, req mcp.Request) context.Context {
	params := paramsFromRequest(req)
	if isNilParams(params) {
		return ctx
	}

	meta := params.GetMeta()
	if !hasTraceContextMeta(meta) {
		return ctx
	}

	extractedCtx := requestMetaPropagator.Extract(ctx, requestMetaCarrier{meta: meta})
	extractedSpanContext := trace.SpanContextFromContext(extractedCtx)
	if extractedSpanContext.IsValid() {
		// Preserve request cancellation/deadlines while overriding span parent.
		ctx = trace.ContextWithRemoteSpanContext(ctx, extractedSpanContext)
	}

	ctx = mergeBaggageFromContexts(ctx, extractedCtx)

	return ctx
}

func mergeBaggageFromContexts(baseCtx, extractedCtx context.Context) context.Context {
	extractedBag := baggage.FromContext(extractedCtx)
	if extractedBag.Len() == 0 {
		return baseCtx
	}
	baseBag := baggage.FromContext(baseCtx)
	if baseBag.Len() == 0 {
		return baggage.ContextWithBaggage(baseCtx, extractedBag)
	}

	mergedBag := baseBag
	for _, member := range extractedBag.Members() {
		// Best-effort merge: if a member cannot be set, keep existing baggage and continue.
		nextBag, err := mergedBag.SetMember(member)
		if err == nil {
			mergedBag = nextBag
		}
	}
	return baggage.ContextWithBaggage(baseCtx, mergedBag)
}

func paramsFromRequest(req mcp.Request) mcp.Params {
	if req == nil {
		return nil
	}
	return req.GetParams()
}

func isNilParams(params mcp.Params) bool {
	return isNil(params)
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

func hasTraceContextMeta(meta map[string]any) bool {
	for _, key := range traceContextMetaKeys {
		if value, ok := meta[key].(string); ok && value != "" {
			return true
		}
	}
	return false
}

type requestMetaCarrier struct {
	meta map[string]any
}

func (carrier requestMetaCarrier) Get(key string) string {
	value, _ := carrier.meta[key].(string)
	return value
}

func (carrier requestMetaCarrier) Set(key, value string) {
	if carrier.meta == nil {
		return
	}
	carrier.meta[key] = value
}

func (carrier requestMetaCarrier) Keys() []string {
	keys := make([]string, 0, len(traceContextMetaKeys))
	for _, key := range traceContextMetaKeys {
		if value, ok := carrier.meta[key].(string); ok && value != "" {
			keys = append(keys, key)
		}
	}
	return keys
}
