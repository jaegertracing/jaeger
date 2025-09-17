package tracestore

import (
	"time"

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
