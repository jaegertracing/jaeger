// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package sqlquery

import (
	"slices"

	ast "github.com/jaegertracing/jaeger-idl/query/expression/v1"
)

// relation is one of the four virtual relations of RFC 0019 §4.1.
type relation string

const (
	relationSpans  relation = "spans"
	relationEvents relation = "events"
	relationLinks  relation = "links"
	relationTraces relation = "traces"
)

// column is what a column name resolves to. A column with a Field names a built-in field of
// its level; one without is the level's attribute map, addressed by subscript.
type column struct {
	Level ast.Level
	Field string
}

func (c column) isMap() bool { return c.Field == "" }

// The column tables are keyed by the exact snake_case spelling of the dialect. A name in another
// case or spelling is not a column.
var spansColumns = map[string]column{
	"trace_id":            {Level: ast.LevelSpan, Field: ast.SpanFieldTraceID},
	"span_id":             {Level: ast.LevelSpan, Field: ast.SpanFieldSpanID},
	"parent_span_id":      {Level: ast.LevelSpan, Field: ast.SpanFieldParentSpanID},
	"trace_state":         {Level: ast.LevelSpan, Field: ast.SpanFieldTraceState},
	"name":                {Level: ast.LevelSpan, Field: ast.SpanFieldName},
	"kind":                {Level: ast.LevelSpan, Field: ast.SpanFieldKind},
	"start_time":          {Level: ast.LevelSpan, Field: ast.SpanFieldStartTime},
	"end_time":            {Level: ast.LevelSpan, Field: ast.SpanFieldEndTime},
	"duration":            {Level: ast.LevelSpan, Field: ast.SpanFieldDuration},
	"status":              {Level: ast.LevelSpan, Field: ast.SpanFieldStatus},
	"status_message":      {Level: ast.LevelSpan, Field: ast.SpanFieldStatusMessage},
	"service":             {Level: ast.LevelResource, Field: ast.ResourceFieldService},
	"scope_name":          {Level: ast.LevelScope, Field: ast.ScopeFieldName},
	"scope_version":       {Level: ast.LevelScope, Field: ast.ScopeFieldVersion},
	"attributes":          {Level: ast.LevelSpan},
	"resource_attributes": {Level: ast.LevelResource},
	"scope_attributes":    {Level: ast.LevelScope},
}

var eventsColumns = map[string]column{
	"name":             {Level: ast.LevelEvent, Field: ast.EventFieldName},
	"time":             {Level: ast.LevelEvent, Field: ast.EventFieldTime},
	"time_since_start": {Level: ast.LevelEvent, Field: ast.EventFieldTimeSinceStart},
	"attributes":       {Level: ast.LevelEvent},
}

var linksColumns = map[string]column{
	"linked_trace_id": {Level: ast.LevelLink, Field: ast.LinkFieldTraceID},
	"linked_span_id":  {Level: ast.LevelLink, Field: ast.LinkFieldSpanID},
	"trace_state":     {Level: ast.LevelLink, Field: ast.LinkFieldTraceState},
	"attributes":      {Level: ast.LevelLink},
}

// tracesColumns are the RFC 0011 summary fields. They have no expression-level counterpart yet,
// so a traces column may be selected but not filtered on (RFC 0019 §4.4).
var tracesColumns = []string{"trace_id", "root_service", "root_name", "start_time", "duration", "span_count", "error_count"}

// correlationColumns are the columns of events and links that tie a row to its span. They are
// not predicates: the EXISTS shape of RFC 0019 §4.3 consumes them.
var correlationColumns = []string{"span_id", "trace_id"}

// columnsOf returns the column table of a relation that predicates may read.
func columnsOf(rel relation) map[string]column {
	switch rel {
	case relationSpans:
		return spansColumns
	case relationEvents:
		return eventsColumns
	case relationLinks:
		return linksColumns
	default:
		return nil
	}
}

// columnNames lists a relation's columns in the dialect's snake_case spelling, for a hint.
func columnNames(rel relation) string {
	switch rel {
	case relationSpans:
		return "trace_id, span_id, parent_span_id, name, kind, start_time, end_time, duration, status, status_message, trace_state, service, scope_name, scope_version, attributes[...], resource_attributes[...], scope_attributes[...]"
	case relationEvents:
		return "name, time, time_since_start, attributes[...]"
	case relationLinks:
		return "linked_trace_id, linked_span_id, trace_state, attributes[...]"
	default:
		return "trace_id, root_service, root_name, start_time, duration, span_count, error_count"
	}
}

func isTracesColumn(name string) bool {
	return slices.Contains(tracesColumns, name)
}

func isCorrelationColumn(name string) bool {
	return slices.Contains(correlationColumns, name)
}

// sortableColumns are the span fields RFC 0016 §6 declares sortable.
var sortableColumns = []string{"start_time", "duration", "trace_id", "span_id"}

func isSortableColumn(name string) bool {
	return slices.Contains(sortableColumns, name)
}
