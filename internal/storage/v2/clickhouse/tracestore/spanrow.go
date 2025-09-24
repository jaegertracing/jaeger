// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore/dbmodel"
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

func (sr *spanRow) ToDBModel() dbmodel.Span {
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
	return span.ToDBModel(), nil
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
