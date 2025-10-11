// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"encoding/base64"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore/dbmodel"
	"github.com/jaegertracing/jaeger/internal/telemetry/otelsemconv"
)

type spanRow struct {
	id                          string
	traceID                     string
	traceState                  string
	parentSpanID                string
	name                        string
	kind                        string
	startTime                   time.Time
	statusCode                  string
	statusMessage               string
	rawDuration                 int64
	boolAttributeKeys           []string
	boolAttributeValues         []bool
	doubleAttributeKeys         []string
	doubleAttributeValues       []float64
	intAttributeKeys            []string
	intAttributeValues          []int64
	strAttributeKeys            []string
	strAttributeValues          []string
	complexAttributeKeys        []string
	complexAttributeValues      []string
	eventNames                  []string
	eventTimestamps             []time.Time
	eventBoolAttributeKeys      [][]string
	eventBoolAttributeValues    [][]bool
	eventDoubleAttributeKeys    [][]string
	eventDoubleAttributeValues  [][]float64
	eventIntAttributeKeys       [][]string
	eventIntAttributeValues     [][]int64
	eventStrAttributeKeys       [][]string
	eventStrAttributeValues     [][]string
	eventComplexAttributeKeys   [][]string
	eventComplexAttributeValues [][]string
	linkTraceIDs                []string
	linkSpanIDs                 []string
	linkTraceStates             []string
	serviceName                 string
	scopeName                   string
	scopeVersion                string
}

func (sr *spanRow) toDBModel() dbmodel.Span {
	return dbmodel.Span{
		ID:            sr.id,
		TraceID:       sr.traceID,
		TraceState:    sr.traceState,
		ParentSpanID:  sr.parentSpanID,
		Name:          sr.name,
		Kind:          sr.kind,
		StartTime:     sr.startTime,
		StatusCode:    sr.statusCode,
		StatusMessage: sr.statusMessage,
		Duration:      time.Duration(sr.rawDuration),
		Attributes: dbmodel.Attributes{
			BoolAttributes:    zipAttributes(sr.boolAttributeKeys, sr.boolAttributeValues),
			DoubleAttributes:  zipAttributes(sr.doubleAttributeKeys, sr.doubleAttributeValues),
			IntAttributes:     zipAttributes(sr.intAttributeKeys, sr.intAttributeValues),
			StrAttributes:     zipAttributes(sr.strAttributeKeys, sr.strAttributeValues),
			ComplexAttributes: zipAttributes(sr.complexAttributeKeys, sr.complexAttributeValues),
		},
		Events: buildEvents(
			sr.eventNames,
			sr.eventTimestamps,
			sr.eventBoolAttributeKeys, sr.eventBoolAttributeValues,
			sr.eventDoubleAttributeKeys, sr.eventDoubleAttributeValues,
			sr.eventIntAttributeKeys, sr.eventIntAttributeValues,
			sr.eventStrAttributeKeys, sr.eventStrAttributeValues,
			sr.eventComplexAttributeKeys, sr.eventComplexAttributeValues,
		),
		Links:        buildLinks(sr.linkTraceIDs, sr.linkSpanIDs, sr.linkTraceStates),
		ServiceName:  sr.serviceName,
		ScopeName:    sr.scopeName,
		ScopeVersion: sr.scopeVersion,
	}
}

func scanSpanRow(rows driver.Rows) (dbmodel.Span, error) {
	var span spanRow
	err := rows.Scan(
		&span.id,
		&span.traceID,
		&span.traceState,
		&span.parentSpanID,
		&span.name,
		&span.kind,
		&span.startTime,
		&span.statusCode,
		&span.statusMessage,
		&span.rawDuration,
		&span.boolAttributeKeys,
		&span.boolAttributeValues,
		&span.doubleAttributeKeys,
		&span.doubleAttributeValues,
		&span.intAttributeKeys,
		&span.intAttributeValues,
		&span.strAttributeKeys,
		&span.strAttributeValues,
		&span.complexAttributeKeys,
		&span.complexAttributeValues,
		&span.eventNames,
		&span.eventTimestamps,
		&span.eventBoolAttributeKeys,
		&span.eventBoolAttributeValues,
		&span.eventDoubleAttributeKeys,
		&span.eventDoubleAttributeValues,
		&span.eventIntAttributeKeys,
		&span.eventIntAttributeValues,
		&span.eventStrAttributeKeys,
		&span.eventStrAttributeValues,
		&span.eventComplexAttributeKeys,
		&span.eventComplexAttributeValues,
		&span.linkTraceIDs,
		&span.linkSpanIDs,
		&span.linkTraceStates,
		&span.serviceName,
		&span.scopeName,
		&span.scopeVersion,
	)
	if err != nil {
		return dbmodel.Span{}, err
	}
	return span.toDBModel(), nil
}

func zipAttributes[T any](keys []string, values []T) []dbmodel.Attribute[T] {
	n := len(keys)
	attrs := make([]dbmodel.Attribute[T], n)
	for i := 0; i < n; i++ {
		attrs[i] = dbmodel.Attribute[T]{Key: keys[i], Value: values[i]}
	}
	return attrs
}

func buildEvents(
	names []string,
	timestamps []time.Time,
	boolAttributeKeys [][]string, boolAttributeValues [][]bool,
	doubleAttributeKeys [][]string, doubleAttributeValues [][]float64,
	intAttributeKeys [][]string, intAttributeValues [][]int64,
	strAttributeKeys [][]string, strAttributeValues [][]string,
	complexAttributeKeys [][]string, complexAttributeValues [][]string,
) []dbmodel.Event {
	var events []dbmodel.Event
	for i := 0; i < len(names) && i < len(timestamps); i++ {
		event := dbmodel.Event{
			Name:      names[i],
			Timestamp: timestamps[i],
			Attributes: dbmodel.Attributes{
				BoolAttributes:    zipAttributes(boolAttributeKeys[i], boolAttributeValues[i]),
				DoubleAttributes:  zipAttributes(doubleAttributeKeys[i], doubleAttributeValues[i]),
				IntAttributes:     zipAttributes(intAttributeKeys[i], intAttributeValues[i]),
				StrAttributes:     zipAttributes(strAttributeKeys[i], strAttributeValues[i]),
				ComplexAttributes: zipAttributes(complexAttributeKeys[i], complexAttributeValues[i]),
			},
		}
		events = append(events, event)
	}
	return events
}

func buildLinks(traceIDs, spanIDs, states []string) []dbmodel.Link {
	var links []dbmodel.Link
	for i := 0; i < len(traceIDs) && i < len(spanIDs) && i < len(states); i++ {
		links = append(links, dbmodel.Link{
			TraceID:    traceIDs[i],
			SpanID:     spanIDs[i],
			TraceState: states[i],
		})
	}
	return links
}

func spanToRow(
	resource pcommon.Resource,
	scope pcommon.InstrumentationScope,
	span ptrace.Span,
) spanRow {
	// we assume a sanitizer was applied upstream to guarantee non-empty service name
	serviceName, _ := resource.Attributes().Get(otelsemconv.ServiceNameKey)
	duration := span.EndTimestamp().AsTime().Sub(span.StartTimestamp().AsTime()).Nanoseconds()
	sr := spanRow{
		id:            span.SpanID().String(),
		traceID:       span.TraceID().String(),
		traceState:    span.TraceState().AsRaw(),
		parentSpanID:  span.ParentSpanID().String(),
		name:          span.Name(),
		kind:          jptrace.SpanKindToString(span.Kind()),
		startTime:     span.StartTimestamp().AsTime(),
		statusCode:    span.Status().Code().String(),
		statusMessage: span.Status().Message(),
		rawDuration:   duration,
		serviceName:   serviceName.Str(),
		scopeName:     scope.Name(),
		scopeVersion:  scope.Version(),
	}
	sr.appendSpanAttributes(span.Attributes())
	for _, event := range span.Events().All() {
		sr.appendEvent(event)
	}

	return sr
}

func (sr *spanRow) appendSpanAttributes(attrs pcommon.Map) {
	a := extractAttributes(attrs)
	sr.boolAttributeKeys = append(sr.boolAttributeKeys, a.boolKeys...)
	sr.boolAttributeValues = append(sr.boolAttributeValues, a.boolValues...)
	sr.doubleAttributeKeys = append(sr.doubleAttributeKeys, a.doubleKeys...)
	sr.doubleAttributeValues = append(sr.doubleAttributeValues, a.doubleValues...)
	sr.intAttributeKeys = append(sr.intAttributeKeys, a.intKeys...)
	sr.intAttributeValues = append(sr.intAttributeValues, a.intValues...)
	sr.strAttributeKeys = append(sr.strAttributeKeys, a.strKeys...)
	sr.strAttributeValues = append(sr.strAttributeValues, a.strValues...)
	sr.complexAttributeKeys = append(sr.complexAttributeKeys, a.complexKeys...)
	sr.complexAttributeValues = append(sr.complexAttributeValues, a.complexValues...)
}

func (sr *spanRow) appendEvent(event ptrace.SpanEvent) {
	sr.eventNames = append(sr.eventNames, event.Name())
	sr.eventTimestamps = append(sr.eventTimestamps, event.Timestamp().AsTime())

	evAttrs := extractAttributes(event.Attributes())
	sr.eventBoolAttributeKeys = append(sr.eventBoolAttributeKeys, evAttrs.boolKeys)
	sr.eventBoolAttributeValues = append(sr.eventBoolAttributeValues, evAttrs.boolValues)
	sr.eventDoubleAttributeKeys = append(sr.eventDoubleAttributeKeys, evAttrs.doubleKeys)
	sr.eventDoubleAttributeValues = append(sr.eventDoubleAttributeValues, evAttrs.doubleValues)
	sr.eventIntAttributeKeys = append(sr.eventIntAttributeKeys, evAttrs.intKeys)
	sr.eventIntAttributeValues = append(sr.eventIntAttributeValues, evAttrs.intValues)
	sr.eventStrAttributeKeys = append(sr.eventStrAttributeKeys, evAttrs.strKeys)
	sr.eventStrAttributeValues = append(sr.eventStrAttributeValues, evAttrs.strValues)
	sr.eventComplexAttributeKeys = append(sr.eventComplexAttributeKeys, evAttrs.complexKeys)
	sr.eventComplexAttributeValues = append(sr.eventComplexAttributeValues, evAttrs.complexValues)
}

func extractAttributes(attrs pcommon.Map) (out struct {
	boolKeys      []string
	boolValues    []bool
	doubleKeys    []string
	doubleValues  []float64
	intKeys       []string
	intValues     []int64
	strKeys       []string
	strValues     []string
	complexKeys   []string
	complexValues []string
}) {
	attrs.Range(func(k string, v pcommon.Value) bool {
		switch v.Type() {
		case pcommon.ValueTypeBool:
			out.boolKeys = append(out.boolKeys, k)
			out.boolValues = append(out.boolValues, v.Bool())
		case pcommon.ValueTypeDouble:
			out.doubleKeys = append(out.doubleKeys, k)
			out.doubleValues = append(out.doubleValues, v.Double())
		case pcommon.ValueTypeInt:
			out.intKeys = append(out.intKeys, k)
			out.intValues = append(out.intValues, v.Int())
		case pcommon.ValueTypeStr:
			out.strKeys = append(out.strKeys, k)
			out.strValues = append(out.strValues, v.Str())
		case pcommon.ValueTypeBytes:
			key := "@bytes@" + k
			encoded := base64.StdEncoding.EncodeToString(v.Bytes().AsRaw())
			out.complexKeys = append(out.complexKeys, key)
			out.complexValues = append(out.complexValues, encoded)
		case pcommon.ValueTypeSlice, pcommon.ValueTypeMap:
			// TODO
		default:
		}
		return true
	})
	return out
}
