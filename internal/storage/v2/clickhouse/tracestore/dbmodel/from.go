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

// FromRow converts a ClickHouse stored span row to an OpenTelemetry Traces object.
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
	rs := convertResource(storedSpan, span)
	rs.CopyTo(resource)

	scope := scopeSpans.Scope()
	sc := convertScope(storedSpan, span)
	sc.CopyTo(scope)

	return trace
}

func convertResource(sr *SpanRow, spanForWarnings ptrace.Span) pcommon.Resource {
	resource := ptrace.NewResourceSpans().Resource()
	resource.Attributes().PutStr(otelsemconv.ServiceNameKey, sr.ServiceName)
	putAttributes(
		resource.Attributes(),
		&sr.ResourceAttributes,
		spanForWarnings,
	)
	return resource
}

func convertScope(sr *SpanRow, spanForWarnings ptrace.Span) pcommon.InstrumentationScope {
	scope := ptrace.NewScopeSpans().Scope()
	scope.SetName(sr.ScopeName)
	scope.SetVersion(sr.ScopeVersion)
	putAttributes(
		scope.Attributes(),
		&sr.ScopeAttributes,
		spanForWarnings,
	)

	return scope
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
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(sr.StartTime.Add(time.Duration(sr.Duration))))
	span.Status().SetCode(jptrace.StringToStatusCode(sr.StatusCode))
	span.Status().SetMessage(sr.StatusMessage)

	putAttributes(
		span.Attributes(),
		&sr.Attributes,
		span,
	)

	for i, e := range sr.EventNames {
		event := span.Events().AppendEmpty()
		event.SetName(e)
		event.SetTimestamp(pcommon.NewTimestampFromTime(sr.EventTimestamps[i]))
		putAttributes(
			event.Attributes(),
			&Attributes{
				BoolKeys:      sr.EventAttributes.BoolKeys[i],
				BoolValues:    sr.EventAttributes.BoolValues[i],
				DoubleKeys:    sr.EventAttributes.DoubleKeys[i],
				DoubleValues:  sr.EventAttributes.DoubleValues[i],
				IntKeys:       sr.EventAttributes.IntKeys[i],
				IntValues:     sr.EventAttributes.IntValues[i],
				StrKeys:       sr.EventAttributes.StrKeys[i],
				StrValues:     sr.EventAttributes.StrValues[i],
				ComplexKeys:   sr.EventAttributes.ComplexKeys[i],
				ComplexValues: sr.EventAttributes.ComplexValues[i],
			},
			span,
		)
	}

	for i, l := range sr.LinkTraceIDs {
		link := span.Links().AppendEmpty()
		traceID, err := hex.DecodeString(l)
		if err != nil {
			jptrace.AddWarnings(span, fmt.Sprintf("failed to decode link trace ID: %v", err))
			continue
		}
		link.SetTraceID(pcommon.TraceID(traceID))
		spanID, err := hex.DecodeString(sr.LinkSpanIDs[i])
		if err != nil {
			jptrace.AddWarnings(span, fmt.Sprintf("failed to decode link span ID: %v", err))
			continue
		}
		link.SetSpanID(pcommon.SpanID(spanID))
		link.TraceState().FromRaw(sr.LinkTraceStates[i])

		putAttributes(
			link.Attributes(),
			&Attributes{
				BoolKeys:      sr.LinkAttributes.BoolKeys[i],
				BoolValues:    sr.LinkAttributes.BoolValues[i],
				DoubleKeys:    sr.LinkAttributes.DoubleKeys[i],
				DoubleValues:  sr.LinkAttributes.DoubleValues[i],
				IntKeys:       sr.LinkAttributes.IntKeys[i],
				IntValues:     sr.LinkAttributes.IntValues[i],
				StrKeys:       sr.LinkAttributes.StrKeys[i],
				StrValues:     sr.LinkAttributes.StrValues[i],
				ComplexKeys:   sr.LinkAttributes.ComplexKeys[i],
				ComplexValues: sr.LinkAttributes.ComplexValues[i],
			},
			span,
		)
	}

	return span, nil
}

func putAttributes(
	attrs pcommon.Map,
	storedAttrs *Attributes,
	spanForWarnings ptrace.Span,
) {
	for i := 0; i < len(storedAttrs.BoolKeys); i++ {
		attrs.PutBool(storedAttrs.BoolKeys[i], storedAttrs.BoolValues[i])
	}
	for i := 0; i < len(storedAttrs.DoubleKeys); i++ {
		attrs.PutDouble(storedAttrs.DoubleKeys[i], storedAttrs.DoubleValues[i])
	}
	for i := 0; i < len(storedAttrs.IntKeys); i++ {
		attrs.PutInt(storedAttrs.IntKeys[i], storedAttrs.IntValues[i])
	}
	for i := 0; i < len(storedAttrs.StrKeys); i++ {
		attrs.PutStr(storedAttrs.StrKeys[i], storedAttrs.StrValues[i])
	}
	for i := 0; i < len(storedAttrs.ComplexKeys); i++ {
		if strings.HasPrefix(storedAttrs.ComplexKeys[i], "@bytes@") {
			decoded, err := base64.StdEncoding.DecodeString(storedAttrs.ComplexValues[i])
			if err != nil {
				jptrace.AddWarnings(spanForWarnings, fmt.Sprintf("failed to decode bytes attribute %q: %s", storedAttrs.ComplexKeys[i], err.Error()))
				continue
			}
			k := strings.TrimPrefix(storedAttrs.ComplexKeys[i], "@bytes@")
			attrs.PutEmptyBytes(k).FromRaw(decoded)
		}
	}
}
