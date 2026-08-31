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
	"go.opentelemetry.io/collector/pdata/xpdata"

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

// decodeTraceID decodes a hex string into a pcommon.TraceID, validating that
// it contains exactly 16 bytes so the conversion cannot panic on corrupted rows.
func decodeTraceID(s string) (pcommon.TraceID, error) {
	var id pcommon.TraceID
	b, err := hex.DecodeString(s)
	if err != nil {
		return id, err
	}
	if len(b) != len(id) {
		return id, fmt.Errorf("invalid length %d of decoded trace ID %q, expected %d bytes", len(b), s, len(id))
	}
	copy(id[:], b)
	return id, nil
}

// decodeSpanID decodes a hex string into a pcommon.SpanID, validating that
// it contains exactly 8 bytes so the conversion cannot panic on corrupted rows.
func decodeSpanID(s string) (pcommon.SpanID, error) {
	var id pcommon.SpanID
	b, err := hex.DecodeString(s)
	if err != nil {
		return id, err
	}
	if len(b) != len(id) {
		return id, fmt.Errorf("invalid length %d of decoded span ID %q, expected %d bytes", len(b), s, len(id))
	}
	copy(id[:], b)
	return id, nil
}

func convertSpan(sr *SpanRow) (ptrace.Span, error) {
	span := ptrace.NewSpan()
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(sr.StartTime))
	traceId, err := decodeTraceID(sr.TraceID)
	if err != nil {
		return span, fmt.Errorf("failed to decode trace ID: %w", err)
	}
	span.SetTraceID(traceId)
	spanId, err := decodeSpanID(sr.ID)
	if err != nil {
		return span, fmt.Errorf("failed to decode span ID: %w", err)
	}
	span.SetSpanID(spanId)
	if sr.ParentSpanID != "" {
		parentSpanId, err := decodeSpanID(sr.ParentSpanID)
		if err != nil {
			return span, fmt.Errorf("failed to decode parent span ID: %w", err)
		}
		span.SetParentSpanID(parentSpanId)
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
		putAttributes2D(event.Attributes(), &sr.EventAttributes, i, span)
	}

	for i, l := range sr.LinkTraceIDs {
		link := span.Links().AppendEmpty()
		traceID, err := decodeTraceID(l)
		if err != nil {
			jptrace.AddWarnings(span, fmt.Sprintf("failed to decode link trace ID: %v", err))
			continue
		}
		link.SetTraceID(traceID)
		spanID, err := decodeSpanID(sr.LinkSpanIDs[i])
		if err != nil {
			jptrace.AddWarnings(span, fmt.Sprintf("failed to decode link span ID: %v", err))
			continue
		}
		link.SetSpanID(spanID)
		link.TraceState().FromRaw(sr.LinkTraceStates[i])

		putAttributes2D(link.Attributes(), &sr.LinkAttributes, i, span)
	}

	return span, nil
}

func putAttributes2D(
	attrs pcommon.Map,
	storedAttrs *Attributes2D,
	idx int,
	spanForWarnings ptrace.Span,
) {
	putAttributes(
		attrs,
		&Attributes{
			BoolKeys:      storedAttrs.BoolKeys[idx],
			BoolValues:    storedAttrs.BoolValues[idx],
			DoubleKeys:    storedAttrs.DoubleKeys[idx],
			DoubleValues:  storedAttrs.DoubleValues[idx],
			IntKeys:       storedAttrs.IntKeys[idx],
			IntValues:     storedAttrs.IntValues[idx],
			StrKeys:       storedAttrs.StrKeys[idx],
			StrValues:     storedAttrs.StrValues[idx],
			ComplexKeys:   storedAttrs.ComplexKeys[idx],
			ComplexValues: storedAttrs.ComplexValues[idx],
		},
		spanForWarnings,
	)
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
		switch {
		case strings.HasPrefix(storedAttrs.ComplexKeys[i], "@bytes@"):
			decoded, err := base64.StdEncoding.DecodeString(storedAttrs.ComplexValues[i])
			if err != nil {
				jptrace.AddWarnings(spanForWarnings, fmt.Sprintf("failed to decode bytes attribute %q: %s", storedAttrs.ComplexKeys[i], err.Error()))
				continue
			}
			k := strings.TrimPrefix(storedAttrs.ComplexKeys[i], "@bytes@")
			attrs.PutEmptyBytes(k).FromRaw(decoded)
		case strings.HasPrefix(storedAttrs.ComplexKeys[i], "@slice@"):
			k := strings.TrimPrefix(storedAttrs.ComplexKeys[i], "@slice@")
			m := &xpdata.JSONUnmarshaler{}
			val, err := m.UnmarshalValue([]byte(storedAttrs.ComplexValues[i]))
			if err != nil {
				jptrace.AddWarnings(
					spanForWarnings,
					fmt.Sprintf(
						"failed to unmarshal slice attribute %q: %s",
						storedAttrs.ComplexKeys[i],
						err.Error(),
					),
				)
				continue
			}
			attrs.PutEmptySlice(k).FromRaw(val.Slice().AsRaw())
		case strings.HasPrefix(storedAttrs.ComplexKeys[i], "@map@"):
			k := strings.TrimPrefix(storedAttrs.ComplexKeys[i], "@map@")
			m := &xpdata.JSONUnmarshaler{}
			val, err := m.UnmarshalValue([]byte(storedAttrs.ComplexValues[i]))
			if err != nil {
				jptrace.AddWarnings(
					spanForWarnings,
					fmt.Sprintf(
						"failed to unmarshal map attribute %q: %s",
						storedAttrs.ComplexKeys[i],
						err.Error(),
					),
				)
				continue
			}
			attrs.PutEmptyMap(k).FromRaw(val.Map().AsRaw())
		default:
			jptrace.AddWarnings(
				spanForWarnings,
				fmt.Sprintf("unsupported complex attribute key: %q", storedAttrs.ComplexKeys[i]),
			)
		}
	}
}
