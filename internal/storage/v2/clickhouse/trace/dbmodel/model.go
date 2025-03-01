// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"encoding/hex"
	"errors"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

type Model struct {
	Timestamp                time.Time
	TraceId                  string
	SpanId                   string
	ParentSpanId             string
	TraceState               string
	SpanName                 string
	SpanKind                 string
	ServiceName              string
	ResourceAttributesKeys   []string `ch:"ResourceAttributes.keys"`
	ResourceAttributesValues []string `ch:"ResourceAttributes.values"`
	ScopeName                string
	ScopeVersion             string
	SpanAttributesKeys       []string `ch:"SpanAttributes.keys"`
	SpanAttributesValues     []string `ch:"SpanAttributes.values"`
	Duration                 uint64
	StatusCode               string
	StatusMessage            string
	EventsTimestamp          []time.Time         `ch:"Events.Timestamp"`
	EventsName               []string            `ch:"Events.Name"`
	EventsAttributes         []map[string]string `ch:"Events.Attributes"`
	LinksTraceId             []string            `ch:"Links.TraceId"`
	LinksSpanId              []string            `ch:"Links.SpanId"`
	LinksTraceState          []string            `ch:"Links.TraceState"`
	LinksAttributes          []map[string]string `ch:"Links.Attributes"`
}

// ConvertToTraces convert the db model read from clickhouse to OTel Traces.
func (m Model) ConvertToTraces() (ptrace.Traces, error) {
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	m.covertResourceSpans(rs)

	scopeSpans := rs.ScopeSpans().AppendEmpty()
	scopeSpans.SetSchemaUrl("https://opentelemetry.io/schemas/1.7.0")
	m.covertInstrumentationScope(scopeSpans.Scope())

	span := scopeSpans.Spans().AppendEmpty()
	err := m.covertSpan(span)
	if err != nil {
		return traces, err
	}

	return traces, nil
}

func (m Model) covertResourceSpans(spans ptrace.ResourceSpans) {
	spans.SetSchemaUrl("https://opentelemetry.io/schemas/1.4.0")
	spans.Resource().SetDroppedAttributesCount(10)

	for i := 0; i < len(m.ResourceAttributesKeys); i++ {
		if i < len(m.ResourceAttributesValues) {
			spans.Resource().Attributes().PutStr(m.ResourceAttributesKeys[i], m.ResourceAttributesValues[i])
		}
	}
}

func (m Model) covertInstrumentationScope(scope pcommon.InstrumentationScope) {
	scope.SetName(m.ScopeName)
	scope.SetVersion(m.ScopeVersion)
	scope.SetDroppedAttributesCount(20)
	scope.Attributes().PutStr("lib", "clickhouse")
}

func (m Model) covertSpan(span ptrace.Span) error {
	traceId, err := hex.DecodeString(m.TraceId)
	if err != nil {
		return err
	}
	span.SetTraceID(pcommon.TraceID(traceId))
	spanId, err := hex.DecodeString(m.SpanId)
	if err != nil {
		return err
	}
	span.SetSpanID(pcommon.SpanID(spanId))
	span.TraceState().FromRaw(m.TraceState)

	if m.ParentSpanId != "" {
		parentSpanId, err := hex.DecodeString(m.ParentSpanId)
		if err != nil {
			return err
		}
		span.SetParentSpanID(pcommon.SpanID(parentSpanId))
	}
	span.SetName(m.SpanName)
	span.SetKind(m.spanKind())
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(m.Timestamp))
	//nolint: gosec // G115
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(m.Timestamp.Add(time.Duration(m.Duration))))

	for i := 0; i < len(m.SpanAttributesKeys); i++ {
		if i < len(m.SpanAttributesValues) {
			span.Attributes().PutStr(m.SpanAttributesKeys[i], m.SpanAttributesValues[i])
		}
	}

	span.Status().SetMessage(m.StatusMessage)
	span.Status().SetCode(m.statusCode())

	for i := 0; i < len(m.EventsName); i++ {
		if i < len(m.EventsTimestamp) {
			event := span.Events().AppendEmpty()
			m.covertEvent(event, i)
		}
	}

	for i := 0; i < len(m.LinksSpanId); i++ {
		if i < len(m.LinksTraceId) {
			link := span.Links().AppendEmpty()
			err := m.covertLink(link, i)
			if err != nil {
				return err
			}
		}
	}
	return err
}

func (m Model) covertLink(link ptrace.SpanLink, i int) error {
	var err error
	linksTraceId, linksSpanId := m.LinksTraceId[i], m.LinksSpanId[i]

	if linksTraceId == "" || linksSpanId == "" {
		return errors.New("invalid trace id or span id")
	}

	traceId, err := hex.DecodeString(linksTraceId)
	if err != nil {
		return err
	}
	link.SetTraceID(pcommon.TraceID(traceId))

	spanId, err := hex.DecodeString(linksSpanId)
	if err != nil {
		return err
	}
	link.SetSpanID(pcommon.SpanID(spanId))

	link.TraceState().FromRaw(m.LinksTraceState[i])
	if i < len(m.LinksAttributes) {
		for k, v := range m.LinksAttributes[i] {
			link.Attributes().PutStr(k, v)
		}
	}
	return err
}

func (m Model) covertEvent(event ptrace.SpanEvent, i int) {
	event.SetName(m.EventsName[i])
	event.SetTimestamp(pcommon.NewTimestampFromTime(m.EventsTimestamp[i]))
	if i < len(m.EventsAttributes) {
		eventArt := m.EventsAttributes[i]
		for k, v := range eventArt {
			event.Attributes().PutStr(k, v)
		}
	}
}

func (m Model) statusCode() ptrace.StatusCode {
	switch m.StatusCode {
	case "Error":
		return ptrace.StatusCodeError
	case "Unset":
		return ptrace.StatusCodeUnset
	case "Ok":
		return ptrace.StatusCodeOk
	}
	return -1
}

func (m Model) spanKind() ptrace.SpanKind {
	switch m.SpanKind {
	case "UNSPECIFIED":
		return ptrace.SpanKindUnspecified
	case "Internal":
		return ptrace.SpanKindInternal
	case "Service":
		return ptrace.SpanKindServer
	case "Client":
		return ptrace.SpanKindClient
	case "Producer":
		return ptrace.SpanKindProducer
	case "Consumer":
		return ptrace.SpanKindConsumer
	}
	return -1
}
