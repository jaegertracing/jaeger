// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/jptrace"
)

func FromRow(storedSpan *SpanRow) ptrace.Traces {
	trace := ptrace.NewTraces()
	resourceSpans := trace.ResourceSpans().AppendEmpty()
	scopeSpans := resourceSpans.ScopeSpans().AppendEmpty()
	span := scopeSpans.Spans().AppendEmpty()

	sp, err := convertSpan(storedSpan)
	sp.CopyTo(span)
	if err != nil {
		jptrace.AddWarnings(span, err.Error())
	}

	resource := resourceSpans.Resource()
	rs, err := convertResource(storedSpan)
	if err != nil {
		jptrace.AddWarnings(span, err.Error())
	}
	rs.CopyTo(resource)

	scope := scopeSpans.Scope()
	sc, err := convertScope(storedSpan)
	if err != nil {
		jptrace.AddWarnings(span, err.Error())
	}
	sc.CopyTo(scope)

	return trace
}

func convertResource(*SpanRow) (pcommon.Resource, error) {
	resource := ptrace.NewResourceSpans().Resource()
	// TODO: populate attributes
	// TODO: do we populate the service name from the span?
	return resource, nil
}

func convertScope(s *SpanRow) (pcommon.InstrumentationScope, error) {
	scope := ptrace.NewScopeSpans().Scope()
	scope.SetName(s.ScopeName)
	scope.SetVersion(s.ScopeVersion)
	// TODO: populate attributes

	return scope, nil
}

func convertSpan(sr *SpanRow) (ptrace.Span, error) {
	span := ptrace.NewSpan()
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(sr.StartTime))
	traceId, err := hex.DecodeString(sr.TraceID)
	if err != nil {
		return span, fmt.Errorf("failed to decode trace ID: %w", err)
	}
	span.SetTraceID(pcommon.TraceID(traceId))
	spanId, err := hex.DecodeString(sr.ID)
	if err != nil {
		return span, fmt.Errorf("failed to decode span ID: %w", err)
	}
	span.SetSpanID(pcommon.SpanID(spanId))
	parentSpanId, err := hex.DecodeString(sr.ParentSpanID)
	if err != nil {
		return span, fmt.Errorf("failed to decode parent span ID: %w", err)
	}
	if len(parentSpanId) != 0 {
		span.SetParentSpanID(pcommon.SpanID(parentSpanId))
	}
	span.TraceState().FromRaw(sr.TraceState)
	span.SetName(sr.Name)
	span.SetKind(jptrace.StringToSpanKind(sr.Kind))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(sr.StartTime.Add(time.Duration(sr.RawDuration))))
	span.Status().SetCode(jptrace.StringToStatusCode(sr.StatusCode))
	span.Status().SetMessage(sr.StatusMessage)

	for i := 0; i < len(sr.BoolAttributeKeys); i++ {
		span.Attributes().PutBool(sr.BoolAttributeKeys[i], sr.BoolAttributeValues[i])
	}
	for i := 0; i < len(sr.DoubleAttributeKeys); i++ {
		span.Attributes().PutDouble(sr.DoubleAttributeKeys[i], sr.DoubleAttributeValues[i])
	}
	for i := 0; i < len(sr.IntAttributeKeys); i++ {
		span.Attributes().PutInt(sr.IntAttributeKeys[i], sr.IntAttributeValues[i])
	}
	for i := 0; i < len(sr.StrAttributeKeys); i++ {
		span.Attributes().PutStr(sr.StrAttributeKeys[i], sr.StrAttributeValues[i])
	}
	for i := 0; i < len(sr.ComplexAttributeKeys); i++ {
		key := sr.ComplexAttributeKeys[i]
		if strings.HasPrefix(key, "@bytes@") {
			parsedKey := strings.TrimPrefix(key, "@bytes@")
			decoded, err := base64.StdEncoding.DecodeString(sr.ComplexAttributeValues[i])
			if err != nil {
				jptrace.AddWarnings(span, fmt.Sprintf("failed to decode bytes attribute %q: %s", parsedKey, err.Error()))
				continue
			}
			span.Attributes().PutEmptyBytes(parsedKey).FromRaw(decoded)
		}
	}

	for i, e := range sr.EventNames {
		event := span.Events().AppendEmpty()
		event.SetName(e)
		event.SetTimestamp(pcommon.NewTimestampFromTime(sr.EventTimestamps[i]))
		for j := 0; j < len(sr.EventBoolAttributeKeys[i]); j++ {
			event.Attributes().PutBool(sr.EventBoolAttributeKeys[i][j], sr.EventBoolAttributeValues[i][j])
		}
		for j := 0; j < len(sr.EventDoubleAttributeKeys[i]); j++ {
			event.Attributes().PutDouble(sr.EventDoubleAttributeKeys[i][j], sr.EventDoubleAttributeValues[i][j])
		}
		for j := 0; j < len(sr.EventIntAttributeKeys[i]); j++ {
			event.Attributes().PutInt(sr.EventIntAttributeKeys[i][j], sr.EventIntAttributeValues[i][j])
		}
		for j := 0; j < len(sr.EventStrAttributeKeys[i]); j++ {
			event.Attributes().PutStr(sr.EventStrAttributeKeys[i][j], sr.EventStrAttributeValues[i][j])
		}
		for j := 0; j < len(sr.EventComplexAttributeKeys[i]); j++ {
			if strings.HasPrefix(sr.EventComplexAttributeKeys[i][j], "@bytes@") {
				decoded, err := base64.StdEncoding.DecodeString(sr.EventComplexAttributeValues[i][j])
				if err != nil {
					jptrace.AddWarnings(span, fmt.Sprintf("failed to decode bytes attribute %q: %s", sr.EventComplexAttributeKeys[i][j], err.Error()))
					continue
				}
				k := strings.TrimPrefix(sr.EventComplexAttributeKeys[i][j], "@bytes@")
				event.Attributes().PutEmptyBytes(k).FromRaw(decoded)
			}
		}
	}

	return span, nil
}
