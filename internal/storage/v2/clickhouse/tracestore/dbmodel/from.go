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
	"github.com/jaegertracing/jaeger/internal/telemetry/otelsemconv"
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

func convertResource(sr *SpanRow) (pcommon.Resource, error) {
	resource := ptrace.NewResourceSpans().Resource()
	resource.Attributes().PutStr(otelsemconv.ServiceNameKey, sr.ServiceName)
	// TODO: populate attributes
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

	putAttributes(
		span.Attributes(),
		span,
		sr.BoolAttributeKeys, sr.BoolAttributeValues,
		sr.DoubleAttributeKeys, sr.DoubleAttributeValues,
		sr.IntAttributeKeys, sr.IntAttributeValues,
		sr.StrAttributeKeys, sr.StrAttributeValues,
		sr.ComplexAttributeKeys, sr.ComplexAttributeValues,
	)

	for i, e := range sr.EventNames {
		event := span.Events().AppendEmpty()
		event.SetName(e)
		event.SetTimestamp(pcommon.NewTimestampFromTime(sr.EventTimestamps[i]))
		putAttributes(
			event.Attributes(),
			span,
			sr.EventBoolAttributeKeys[i], sr.EventBoolAttributeValues[i],
			sr.EventDoubleAttributeKeys[i], sr.EventDoubleAttributeValues[i],
			sr.EventIntAttributeKeys[i], sr.EventIntAttributeValues[i],
			sr.EventStrAttributeKeys[i], sr.EventStrAttributeValues[i],
			sr.EventComplexAttributeKeys[i], sr.EventComplexAttributeValues[i],
		)
	}

	for i, l := range sr.LinkTraceIDs {
		link := span.Links().AppendEmpty()
		traceID, err := hex.DecodeString(l)
		if err != nil {
			jptrace.AddWarnings(span, fmt.Sprintf("failed to decode link trace ID: %s", err.Error()))
			continue
		}
		link.SetTraceID(pcommon.TraceID(traceID))
		spanID, err := hex.DecodeString(sr.LinkSpanIDs[i])
		if err != nil {
			jptrace.AddWarnings(span, fmt.Sprintf("failed to decode link span ID: %s", err.Error()))
			continue
		}
		link.SetSpanID(pcommon.SpanID(spanID))
		link.TraceState().FromRaw(sr.LinkTraceStates[i])

		// putAttributes(
		// 	link.Attributes(),
		// 	span,
		// 	sr.LinkBoolAttributeKeys[i], sr.LinkBoolAttributeValues[i],
		// 	sr.LinkDoubleAttributeKeys[i], sr.LinkDoubleAttributeValues[i],
		// 	sr.LinkIntAttributeKeys[i], sr.LinkIntAttributeValues[i],
		// 	sr.LinkStrAttributeKeys[i], sr.LinkStrAttributeValues[i],
		// 	sr.LinkComplexAttributeKeys[i], sr.LinkComplexAttributeValues[i],
		// )
	}

	return span, nil
}

func putAttributes(
	attrs pcommon.Map,
	spanForWarnings ptrace.Span,
	boolKeys []string, boolValues []bool,
	doubleKeys []string, doubleValues []float64,
	intKeys []string, intValues []int64,
	strKeys []string, strValues []string,
	complexKeys []string, complexValues []string,
) {
	for i := 0; i < len(boolKeys); i++ {
		attrs.PutBool(boolKeys[i], boolValues[i])
	}
	for i := 0; i < len(doubleKeys); i++ {
		attrs.PutDouble(doubleKeys[i], doubleValues[i])
	}
	for i := 0; i < len(intKeys); i++ {
		attrs.PutInt(intKeys[i], intValues[i])
	}
	for i := 0; i < len(strKeys); i++ {
		attrs.PutStr(strKeys[i], strValues[i])
	}
	for i := 0; i < len(complexKeys); i++ {
		if strings.HasPrefix(complexKeys[i], "@bytes@") {
			decoded, err := base64.StdEncoding.DecodeString(complexValues[i])
			if err != nil {
				jptrace.AddWarnings(spanForWarnings, fmt.Sprintf("failed to decode bytes attribute %q: %s", complexKeys[i], err.Error()))
				continue
			}
			k := strings.TrimPrefix(complexKeys[i], "@bytes@")
			attrs.PutEmptyBytes(k).FromRaw(decoded)
		}
	}
}
