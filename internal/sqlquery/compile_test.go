// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package sqlquery

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ast "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/internal/expression"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// now is the instant now() evaluates to in every test.
var now = time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)

// window is the conjunct pair every statement needs; the corpus appends its own predicates.
const window = "start_time >= now() - INTERVAL '1' HOUR AND start_time < now()"

// p is the builder the expected trees are written with, so that the test states the tree a Go
// caller would build and the compiler has to match it.
var p expression.Predicate

// finalized puts an expected tree through the same finalization the compiler applies.
func finalized(t *testing.T, call *ast.Call) *ast.Call {
	t.Helper()
	out, err := tracestore.FinalizeFilter(call)
	require.NoError(t, err)
	return out
}

// TestCompileFilters is the accepted corpus of RFC 0019 M0: each statement compiles to the
// tree the builder writes for the same question, byte for byte.
func TestCompileFilters(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		want *ast.Call
	}{
		{
			name: "window only",
			sql:  "SELECT DISTINCT trace_id FROM spans WHERE " + window,
			want: nil,
		},
		{
			name: "service and attribute",
			sql:  "SELECT DISTINCT trace_id FROM spans s WHERE " + window + " AND s.service = 'checkout' AND s.attributes['http.status_code'] >= 500",
			want: p.And(p.Resource().Service.Eq("checkout"), p.Span().Attr("http.status_code").Gte("500")),
		},
		{
			name: "camelCase columns and the IDL spelling of a field",
			sql:  "SELECT DISTINCT traceID FROM spans WHERE startTime >= now() - INTERVAL '1' HOUR AND startTime < now() AND parentSpanId = '' AND statusMessage <> 'ok'",
			want: p.And(p.Span().ParentSpanID.Eq(""), p.Span().StatusMessage.Ne("ok")),
		},
		{
			name: "every comparison operator",
			sql:  "SELECT DISTINCT trace_id FROM spans WHERE " + window + " AND duration > '2s' AND duration < '10s' AND duration >= '1ms' AND duration <= '1us' AND name != 'a' AND name == 'b'",
			want: p.And(
				p.Span().Duration.Gt("2s"), p.Span().Duration.Lt("10s"), p.Span().Duration.Gte("1ms"),
				p.Span().Duration.Lte("1us"), p.Span().Name.Ne("a"), p.Span().Name.Eq("b"),
			),
		},
		{
			name: "a constant on the left is mirrored",
			sql:  "SELECT DISTINCT trace_id FROM spans WHERE " + window + " AND '2s' < duration AND 'x' = name",
			want: p.And(p.Span().Duration.Gt("2s"), p.Span().Name.Eq("x")),
		},
		{
			name: "interval as a duration constant",
			sql:  "SELECT DISTINCT trace_id FROM spans WHERE " + window + " AND duration > INTERVAL '250' MILLISECOND AND duration < INTERVAL 2 SECOND",
			want: p.And(p.Span().Duration.Gt("250ms"), p.Span().Duration.Lt("2s")),
		},
		{
			name: "or, not and parentheses",
			sql:  "SELECT DISTINCT trace_id FROM spans WHERE " + window + " AND (kind = 'server' OR kind = 'client') AND NOT status = 'ok'",
			want: p.And(p.Or(p.Span().Kind.Eq("server"), p.Span().Kind.Eq("client")), p.Not(p.Span().Status.Eq("ok"))),
		},
		{
			name: "in and not in",
			sql:  "SELECT DISTINCT trace_id FROM spans WHERE " + window + " AND kind IN ('server', 'consumer') AND attributes['http.method'] NOT IN ('GET', 'HEAD')",
			want: p.And(p.Span().Kind.In("server", "consumer"), p.Span().Attr("http.method").NotIn("GET", "HEAD")),
		},
		{
			name: "like and not like",
			sql:  "SELECT DISTINCT trace_id FROM spans WHERE " + window + " AND name LIKE '%/api/%' AND name NOT LIKE '%health_check%' AND status_message LIKE 'exact (text)'",
			want: p.And(
				p.Span().Name.Matches(`/api/`),
				p.Not(p.Span().Name.Matches(`health.check`)),
				p.Span().StatusMessage.Eq("exact (text)"),
			),
		},
		{
			name: "regexp and match",
			sql:  "SELECT DISTINCT trace_id FROM spans WHERE " + window + " AND name REGEXP 'GET|POST' AND match(attributes['http.route'], '/users/[0-9]+')",
			want: p.And(p.Span().Name.Matches("GET|POST"), p.Span().Attr("http.route").Matches("/users/[0-9]+")),
		},
		{
			name: "is null and is not null on attributes",
			sql:  "SELECT DISTINCT trace_id FROM spans WHERE " + window + " AND attributes['error'] IS NOT NULL AND resource_attributes['k8s.pod.name'] IS NULL",
			want: p.And(p.Span().Attr("error").Exists(), p.Not(p.Resource().Attr("k8s.pod.name").Exists())),
		},
		{
			name: "phrase and fulltext",
			sql:  "SELECT DISTINCT trace_id FROM spans WHERE " + window + " AND phrase(attributes['db.statement'], 'connection refused') AND fulltext(attributes['exception.message'], 'timeout', 'upstream')",
			want: p.And(p.Span().Attr("db.statement").Phrase("connection", "refused"), p.Span().Attr("exception.message").Fulltext("timeout", "upstream")),
		},
		{
			name: "between",
			sql:  "SELECT DISTINCT trace_id FROM spans WHERE " + window + " AND duration BETWEEN '1s' AND '2s' AND attributes['retries'] NOT BETWEEN 1 AND 3",
			want: p.And(
				p.Span().Duration.Gte("1s"), p.Span().Duration.Lte("2s"),
				p.Not(p.And(p.Span().Attr("retries").Gte("1"), p.Span().Attr("retries").Lte("3"))),
			),
		},
		{
			name: "unqualified attribute, boolean and numbers",
			sql:  "SELECT DISTINCT trace_id FROM spans WHERE " + window + " AND attr('error') = true AND attr('retries') > 3 AND attr('ratio') <= 0.5",
			want: p.And(p.Attr("error").Eq("true"), p.Attr("retries").Gt("3"), p.Attr("ratio").Lte("0.5")),
		},
		{
			name: "two columns compared",
			sql:  "SELECT DISTINCT trace_id FROM spans WHERE " + window + " AND start_time < end_time AND attributes['after'] > attributes['before']",
			want: p.And(p.Span().StartTime.Lt(p.Span().EndTime), p.Span().Attr("after").Gt(p.Span().Attr("before"))),
		},
		{
			name: "a second start_time predicate stays in the filter",
			sql:  "SELECT DISTINCT trace_id FROM spans WHERE " + window + " AND (start_time < '2026-10-23T11:30:00Z' OR duration > '1s')",
			want: p.Or(p.Span().StartTime.Lt("2026-10-23T11:30:00Z"), p.Span().Duration.Gt("1s")),
		},
		{
			name: "scope columns",
			sql:  "SELECT DISTINCT trace_id FROM spans WHERE " + window + " AND scope_name = 'io.opentelemetry.http' AND scope_version LIKE '%1.%' AND scope_attributes['x'] = 'y'",
			want: p.And(p.Scope().Name.Eq("io.opentelemetry.http"), p.Scope().Version.Matches(`1\.`), p.Scope().Attr("x").Eq("y")),
		},
		{
			name: "exists over events",
			sql: "SELECT DISTINCT trace_id FROM spans s WHERE " + window + " AND s.service = 'checkout' AND EXISTS (" +
				"SELECT 1 FROM events e WHERE e.span_id = s.span_id AND e.trace_id = s.trace_id AND e.name = 'exception' AND e.attributes['exception.type'] = 'TimeoutError')",
			want: p.And(
				p.Resource().Service.Eq("checkout"),
				p.Some(p.Event(), p.And(p.Event().Name.Eq("exception"), p.Event().Attr("exception.type").Eq("TimeoutError"))),
			),
		},
		{
			name: "exists over links, correlation last and reversed",
			sql: "SELECT DISTINCT trace_id FROM spans s WHERE " + window + " AND EXISTS (" +
				"SELECT 1 FROM links l WHERE l.linked_trace_id = 'abc' AND l.attributes['time_since'] IS NULL AND s.span_id = l.span_id)",
			want: p.Some(p.Link(), p.And(p.Link().TraceID.Eq("abc"), p.Not(p.Link().Attr("time_since").Exists()))),
		},
		{
			name: "not exists",
			sql:  "SELECT DISTINCT trace_id FROM spans s WHERE " + window + " AND NOT EXISTS (SELECT 1 FROM events e WHERE e.span_id = s.span_id AND e.name = 'retry')",
			want: p.Not(p.Some(p.Event(), p.Event().Name.Eq("retry"))),
		},
		{
			name: "nested boolean structure",
			sql:  "SELECT DISTINCT trace_id FROM spans WHERE " + window + " AND NOT (kind = 'server' OR (status = 'error' AND duration > '1s')) AND ((name = 'a' AND name = 'b') OR name = 'c')",
			want: p.And(
				p.Not(p.Or(p.Span().Kind.Eq("server"), p.And(p.Span().Status.Eq("error"), p.Span().Duration.Gt("1s")))),
				p.Or(p.And(p.Span().Name.Eq("a"), p.Span().Name.Eq("b")), p.Span().Name.Eq("c")),
			),
		},
		{
			name: "start_time predicates that are not bounds stay in the filter",
			sql:  "SELECT DISTINCT trace_id FROM spans WHERE " + window + " AND start_time <> '2026-10-23T11:30:00Z' AND attributes['deadline'] > now() + INTERVAL '30' SECOND",
			want: p.And(p.Span().StartTime.Ne("2026-10-23T11:30:00Z"), p.Span().Attr("deadline").Gt("2026-10-23T12:00:30Z")),
		},
		{
			name: "quoted quote in a literal",
			sql:  "SELECT DISTINCT trace_id FROM spans WHERE " + window + " AND name = 'o''brien' AND status_message = 'a\\'b'",
			want: p.And(p.Span().Name.Eq("o'brien"), p.Span().StatusMessage.Eq("a'b")),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query, err := Compile(tt.sql, now)
			require.NoError(t, err)
			assert.Equal(t, KindFindTraceIDs, query.Kind)
			assert.Equal(t, now.Add(-time.Hour), query.StartTimeMin)
			assert.Equal(t, now, query.StartTimeMax)
			if tt.want == nil {
				assert.Nil(t, query.Filter)
				return
			}
			assert.Equal(t, finalized(t, tt.want), query.Filter)
		})
	}
}

func TestCompileWindow(t *testing.T) {
	tests := []struct {
		name     string
		where    string
		min, max time.Time
	}{
		{
			name:  "instants",
			where: "start_time >= '2026-10-23T10:00:00Z' AND start_time <= '2026-10-23T11:00:00.5Z'",
			min:   now.Add(-2 * time.Hour), max: now.Add(-time.Hour + 500*time.Millisecond),
		},
		{
			name:  "strict lower bound and offset",
			where: "start_time > '2026-10-23T13:00:00+02:00' AND start_time < now()",
			min:   now.Add(-time.Hour), max: now,
		},
		{
			name:  "between",
			where: "start_time BETWEEN now() - INTERVAL 1 DAY AND now() + INTERVAL '5' MINUTE",
			min:   now.Add(-24 * time.Hour), max: now.Add(5 * time.Minute),
		},
		{
			name:  "reversed and qualified",
			where: "now() - INTERVAL '2' WEEK <= s.start_time AND now() > s.start_time",
			min:   now.Add(-14 * 24 * time.Hour), max: now,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query, err := Compile("SELECT DISTINCT trace_id FROM spans s WHERE "+tt.where, now)
			require.NoError(t, err)
			assert.True(t, tt.min.Equal(query.StartTimeMin), "min %s", query.StartTimeMin)
			assert.True(t, tt.max.Equal(query.StartTimeMax), "max %s", query.StartTimeMax)
			assert.Nil(t, query.Filter)
		})
	}
}

func TestCompileShapes(t *testing.T) {
	spanFilter := finalized(t, p.Resource().Service.Eq("checkout"))
	tests := []struct {
		name string
		sql  string
		want Query
	}{
		{
			name: "distinct trace_id is FindTraceIDs",
			sql:  "SELECT DISTINCT trace_id FROM spans WHERE " + window + " AND service = 'checkout' LIMIT 20",
			want: Query{Kind: KindFindTraceIDs, Filter: spanFilter, Limit: 20},
		},
		{
			name: "star is FindSpans",
			sql:  "SELECT * FROM spans WHERE " + window + " AND service = 'checkout' ORDER BY duration DESC, start_time LIMIT 100",
			want: Query{Kind: KindFindSpans, Filter: spanFilter, Limit: 100, OrderBy: []tracestore.SpanSortOrder{
				{Expression: &ast.FieldRef{Name: ast.SpanFieldDuration, Level: ast.LevelSpan}, Direction: tracestore.SortDescending},
				{Expression: &ast.FieldRef{Name: ast.SpanFieldStartTime, Level: ast.LevelSpan}, Direction: tracestore.SortAscending},
			}},
		},
		{
			name: "column list is FindSpans with a projection",
			sql:  "SELECT s.span_id, duration, attributes['http.route'] FROM spans s WHERE " + window + " ORDER BY s.trace_id, s.span_id",
			want: Query{Kind: KindFindSpans, Projection: []ast.Expression{
				&ast.FieldRef{Name: ast.SpanFieldSpanID, Level: ast.LevelSpan},
				&ast.FieldRef{Name: ast.SpanFieldDuration, Level: ast.LevelSpan},
				&ast.AttributeRef{Key: "http.route", Level: ast.LevelSpan},
			}, OrderBy: []tracestore.SpanSortOrder{
				{Expression: &ast.FieldRef{Name: ast.SpanFieldTraceID, Level: ast.LevelSpan}, Direction: tracestore.SortAscending},
				{Expression: &ast.FieldRef{Name: ast.SpanFieldSpanID, Level: ast.LevelSpan}, Direction: tracestore.SortAscending},
			}},
		},
		{
			name: "traces star is FindTraces",
			sql:  "SELECT * FROM traces WHERE trace_id IN (SELECT trace_id FROM spans s WHERE " + window + " AND s.service = 'checkout') LIMIT 5",
			want: Query{Kind: KindFindTraces, Filter: spanFilter, Limit: 5},
		},
		{
			name: "traces columns is FindTraceSummaries",
			sql:  "SELECT t.trace_id, root_service, span_count, error_count FROM traces t WHERE t.trace_id IN (SELECT DISTINCT trace_id FROM spans WHERE " + window + ")",
			want: Query{Kind: KindFindTraceSummaries},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query, err := Compile(tt.sql, now)
			require.NoError(t, err)
			tt.want.StartTimeMin, tt.want.StartTimeMax = now.Add(-time.Hour), now
			tt.want.Normalized = query.Normalized
			assert.Equal(t, &tt.want, query)
		})
	}
}

func TestCompileNormalized(t *testing.T) {
	query, err := Compile("select   distinct trace_id\nfrom spans s where s.start_time >= now() - interval '1' HOUR and s.start_time < now()\n  and s.service='checkout'", now)
	require.NoError(t, err)
	assert.Equal(t, "SELECT DISTINCT trace_id FROM spans AS s WHERE s.start_time >= now() - INTERVAL '1' HOUR AND s.start_time < now() AND s.service = 'checkout'", query.Normalized)
}

// TestRefusals is the refused corpus of RFC 0019 M0: every refusal places itself at the
// construct and says what the dialect admits instead. The message is asserted because it is
// what an agent reads to repair the statement.
func TestRefusals(t *testing.T) {
	const spans = "SELECT DISTINCT trace_id FROM spans s WHERE " + window
	tests := []struct {
		name string
		sql  string
		at   string // the text the refusal points at; "" when the parser places the error at the end
		want string
	}{
		{"syntax error", "SELECT DISTINCT trace_id FROM spans WHERE start_time >= 'a' AND", "", "The statement did not parse as SQL"},
		{"tilde is not an operator", spans + " AND name ~ 'x'", "~", "did not parse"},
		{"timestamp keyword", spans + " AND start_time >= TIMESTAMP '2026-01-01T00:00:00Z'", "TIMESTAMP", "did not parse"},
		{"two statements", "SELECT 1; SELECT 2", "SELECT 1", "the request holds 2 statements"},
		{"not a select", "INSERT INTO spans VALUES (1)", "INSERT", "only a SELECT statement is accepted"},
		{"with", "WITH x AS (SELECT 1) " + spans, "WITH", "WITH is not supported"},
		{"union", spans + " UNION ALL " + spans, "SELECT", "UNION, EXCEPT and INTERSECT are not supported"},
		{"group by", "SELECT service FROM spans WHERE " + window + " GROUP BY service", "GROUP BY", "GROUP BY is not supported yet"},
		{"having", "SELECT service FROM spans WHERE " + window + " GROUP BY service HAVING count() > 1", "GROUP BY", "GROUP BY is not supported yet"},
		{"aggregate in select", "SELECT count(*) FROM spans WHERE " + window, "count", "count() is an aggregate"},
		{"aggregate in where", spans + " AND count() > 1", "count", "count() is an aggregate"},
		{"offset", spans + " LIMIT 10 OFFSET 5", "5", "OFFSET is not supported"},
		{"limit zero", spans + " LIMIT 0", "0", `"0" is not a page size`},
		{"limit not a number", spans + " LIMIT 'ten'", "'ten'", "LIMIT takes a whole number"},
		{"settings", spans + " SETTINGS max_threads = 1", "SETTINGS", "SETTINGS is not supported"},
		{"format", spans + " FORMAT JSON", "FORMAT", "FORMAT is not supported"},
		{"no from", "SELECT 1 WHERE " + window, "SELECT", "FROM is required"},
		{"join", "SELECT DISTINCT a.trace_id FROM spans a JOIN spans b ON a.trace_id = b.trace_id WHERE a." + window, "spans a JOIN", "JOIN is not supported"},
		{"derived table", "SELECT DISTINCT trace_id FROM (SELECT * FROM spans) WHERE " + window, "SELECT * FROM spans)", "FROM takes a relation name"},
		{"unknown relation", "SELECT DISTINCT trace_id FROM logs WHERE " + window, "logs", `unknown relation "logs"`},
		{"events in from", "SELECT DISTINCT trace_id FROM events WHERE " + window, "FROM events", "events is read through EXISTS"},
		{"database qualifier", "SELECT DISTINCT trace_id FROM jaeger.spans WHERE " + window, "jaeger.spans", "a relation has no database qualifier"},
		{"distinct other column", "SELECT DISTINCT name FROM spans WHERE " + window, "SELECT", "DISTINCT is accepted only as SELECT DISTINCT trace_id"},
		{"expression in select", "SELECT duration * 2 FROM spans WHERE " + window, "duration * 2", "SELECT lists columns, or *"},
		{"alias in select", "SELECT duration AS d FROM spans WHERE " + window, "d FROM", "a column alias in SELECT has no effect"},
		{"no where", "SELECT DISTINCT trace_id FROM spans", "SELECT", "WHERE is required"},
		{"no window", "SELECT DISTINCT trace_id FROM spans WHERE service = 'a'", "WHERE", "WHERE needs both bounds on start_time"},
		{"no upper bound", "SELECT DISTINCT trace_id FROM spans WHERE start_time >= now() - INTERVAL '1' HOUR", "WHERE", "WHERE needs an upper bound on start_time"},
		{"no lower bound", "SELECT DISTINCT trace_id FROM spans WHERE start_time < now()", "WHERE", "WHERE needs a lower bound on start_time"},
		{"empty window", "SELECT DISTINCT trace_id FROM spans WHERE start_time >= now() AND start_time < now() - INTERVAL '1' HOUR", "WHERE", "the start_time window is empty"},
		{"bound twice", spans + " AND s.start_time >= now() - INTERVAL '2' HOUR", "s.start_time >= now() - INTERVAL '2'", "start_time is bounded twice on the same side"},
		{"bound not an instant", "SELECT DISTINCT trace_id FROM spans WHERE start_time >= 'yesterday' AND start_time < now()", "'yesterday'", `"yesterday" is not an RFC 3339 instant`},
		{"bound is a number", "SELECT DISTINCT trace_id FROM spans WHERE start_time >= 1700000000 AND start_time < now()", "1700000000", "a start_time bound is an instant"},
		{"now with arguments", "SELECT DISTINCT trace_id FROM spans WHERE start_time >= now('UTC') AND start_time < now()", "now('UTC')", "now() takes no arguments"},
		{"now minus a number", "SELECT DISTINCT trace_id FROM spans WHERE start_time >= now() - 3600 AND start_time < now()", "3600", "only an INTERVAL may be added to or subtracted from now()"},
		{"interval month", "SELECT DISTINCT trace_id FROM spans WHERE start_time >= now() - INTERVAL '1' MONTH AND start_time < now()", "MONTH", `"MONTH" is not an interval unit the dialect knows`},
		{"interval fraction", "SELECT DISTINCT trace_id FROM spans WHERE start_time >= now() - INTERVAL '1.5' HOUR AND start_time < now()", "INTERVAL '1.5'", `"1.5" is not a whole number`},
		{"unknown column", spans + " AND s.operation = 'x'", "s.operation", `spans has no column "operation"`},
		{"unknown alias", spans + " AND t.name = 'x'", "t.name", `unknown alias "t"`},
		{"map without subscript", spans + " AND attributes = 'x'", "attributes =", `"attributes" is a map and is read by subscript`},
		{"subscript on a field", spans + " AND name['x'] = 'y'", "name['x']", `"name" is not a map column and takes no subscript`},
		{"subscript not a string", spans + " AND attributes[1] = 'y'", "[1]", "the subscript of a map column is a quoted attribute key"},
		{"attr with two arguments", spans + " AND attr('a', 'b') = 'y'", "attr('a'", "attr() takes one argument"},
		{"is null on a field", spans + " AND name IS NULL", "name IS NULL", "IS NULL applies to an attribute"},
		{"duration without unit", spans + " AND duration > 2000", "WHERE", "the filter is not valid"},
		{"ordered operator on kind", spans + " AND kind > 'server'", "WHERE", "the filter is not valid"},
		{"two constants", spans + " AND 1 = 1", "1 = 1", "a comparison names a column on at least one side"},
		{"in with a subquery", spans + " AND trace_id IN (SELECT trace_id FROM spans)", "SELECT trace_id FROM spans)", "a subquery in IN is accepted only as"},
		{"in without a list", spans + " AND name IN 'a'", "'a'", "IN takes a parenthesized list of literals"},
		{"like anchored at one end", spans + " AND name LIKE 'GET%'", "'GET%'", `LIKE "GET%" is anchored at one end`},
		{"like anchored at the other end", spans + " AND name LIKE '%GET'", "'%GET'", `LIKE "%GET" is anchored at one end`},
		{"ilike", spans + " AND name ILIKE '%get%'", "name ILIKE", "ILIKE is not supported"},
		{"exists selects star", spans + " AND EXISTS (SELECT * FROM events e WHERE e.span_id = s.span_id AND e.name = 'x')", "FROM events", "did not parse"},
		{"like without a string", spans + " AND name LIKE duration", "duration", "the pattern of LIKE is a quoted string"},
		{"unsupported function", spans + " AND lower(name) = 'x'", "lower(", "the function lower() is not supported"},
		{"arithmetic", spans + " AND duration * 2 > '1s'", "duration * 2", "the operator * is not supported"},
		{"unary minus", spans + " AND -duration > '1s'", "-duration", "the operator - is not supported"},
		{"exists without correlation", spans + " AND EXISTS (SELECT 1 FROM events e WHERE e.name = 'x')", "WHERE e.name", "an EXISTS subquery is tied to its span by e.span_id = s.span_id"},
		{"exists without a predicate", spans + " AND EXISTS (SELECT 1 FROM events e WHERE e.span_id = s.span_id)", "WHERE e.span_id", "an EXISTS subquery needs a predicate on events beside the correlation"},
		{"exists without where", spans + " AND EXISTS (SELECT 1 FROM events e)", "SELECT 1 FROM events e)", "an EXISTS subquery needs WHERE e.span_id = s.span_id"},
		{"exists over spans", spans + " AND EXISTS (SELECT 1 FROM spans x WHERE x.span_id = s.span_id)", "FROM spans x", "EXISTS reads events or links"},
		{"exists reads the outer alias", spans + " AND EXISTS (SELECT 1 FROM events e WHERE e.span_id = s.span_id AND s.name = 'x')", "s.name = 'x'", "a predicate inside EXISTS reads the events columns only"},
		{"exists with a limit", spans + " AND EXISTS (SELECT 1 FROM events e WHERE e.span_id = s.span_id AND e.name = 'x' LIMIT 1)", "SELECT 1 FROM events e WHERE", "an EXISTS subquery takes no ORDER BY or LIMIT"},
		{"correlation on different columns", spans + " AND EXISTS (SELECT 1 FROM events e WHERE e.span_id = s.trace_id AND e.name = 'x')", "e.span_id = s.trace_id", "a correlation ties the same column"},
		{"correlation column as a predicate", spans + " AND EXISTS (SELECT 1 FROM events e WHERE e.span_id = s.span_id AND e.trace_id = 'abc')", "e.trace_id = 'abc'", `"trace_id" is a correlation column, not a predicate`},
		{"nested exists", spans + " AND EXISTS (SELECT 1 FROM events e WHERE e.span_id = s.span_id AND EXISTS (SELECT 1 FROM links l WHERE l.span_id = e.span_id AND l.trace_state = 'x'))", "SELECT 1 FROM links", "EXISTS appears in the WHERE of a spans query"},
		{"order by on trace ids", spans + " ORDER BY duration", "ORDER BY", "ORDER BY applies to a span search"},
		{"order by unsortable", "SELECT * FROM spans WHERE " + window + " ORDER BY name", "name", "ORDER BY takes a sortable column"},
		{"order by attribute", "SELECT * FROM spans WHERE " + window + " ORDER BY attributes['x']", "attributes['x']", "ORDER BY takes a sortable column"},
		{"traces without where", "SELECT * FROM traces", "SELECT", "a traces query is SELECT * FROM traces WHERE trace_id IN"},
		{"traces with a predicate", "SELECT * FROM traces WHERE duration > '5s'", "WHERE", "the WHERE of a traces query is trace_id IN"},
		{"traces in without subquery", "SELECT * FROM traces WHERE trace_id IN ('a', 'b')", "('a'", "trace_id IN takes the subquery"},
		{"traces subquery over events", "SELECT * FROM traces WHERE trace_id IN (SELECT trace_id FROM events WHERE " + window + ")", "FROM events", "the subquery of a traces query reads spans"},
		{"traces subquery selects a span column", "SELECT * FROM traces WHERE trace_id IN (SELECT span_id FROM spans WHERE " + window + ")", "SELECT span_id", "the subquery of a traces query selects trace_id"},
		{"traces subquery with limit", "SELECT * FROM traces WHERE trace_id IN (SELECT trace_id FROM spans WHERE " + window + " LIMIT 5)", "SELECT trace_id", "ORDER BY and LIMIT go on the outer traces query"},
		{"traces order by", "SELECT * FROM traces WHERE trace_id IN (SELECT trace_id FROM spans WHERE " + window + ") ORDER BY duration", "ORDER BY", "a trace search has no ORDER BY"},
		{"traces unknown column", "SELECT trace_id, name FROM traces WHERE trace_id IN (SELECT trace_id FROM spans WHERE " + window + ")", "name", `traces has no column "name"`},
		{"parenthesized query", "(" + spans + ")", "(", "a parenthesized query is not supported"},
		{"having without group by", spans + " HAVING count() > 1", "HAVING", "HAVING is not supported yet"},
		{"window clause", "SELECT * FROM spans WHERE " + window + " WINDOW w AS (PARTITION BY service)", "WINDOW", "window functions are not supported"},
		{"prewhere", "SELECT * FROM spans PREWHERE service = 'a' WHERE " + window, "PREWHERE", "PREWHERE is not supported"},
		{"limit by", spans + " LIMIT 1 BY service", "LIMIT 1 BY", "LIMIT BY is not supported"},
		{"top", "SELECT TOP 5 * FROM spans WHERE " + window, "TOP", "TOP is not supported"},
		{"distinct on", "SELECT DISTINCT ON (trace_id) * FROM spans WHERE " + window, "ON", "DISTINCT ON is not supported"},
		{"table function", "SELECT * FROM numbers(10) WHERE " + window, "numbers", "FROM takes a relation name"},
		{"unknown projected column", "SELECT nonexistent FROM spans WHERE " + window, "nonexistent", `spans has no column "nonexistent"`},
		{"between window with a bad lower bound", "SELECT DISTINCT trace_id FROM spans WHERE start_time BETWEEN 'x' AND now()", "'x'", `"x" is not an RFC 3339 instant`},
		{"between window with a bad upper bound", "SELECT DISTINCT trace_id FROM spans WHERE start_time BETWEEN now() AND 'y'", "'y'", `"y" is not an RFC 3339 instant`},
		{"between window beside a bound", spans + " AND start_time BETWEEN now() - INTERVAL '2' HOUR AND now()", "start_time BETWEEN", "start_time is bounded twice"},
		{"bound is another function", "SELECT DISTINCT trace_id FROM spans WHERE start_time >= today() AND start_time < now()", "today()", "a start_time bound is an instant"},
		{"bound is now times two", "SELECT DISTINCT trace_id FROM spans WHERE start_time >= now() * 2 AND start_time < now()", "now() * 2", "a start_time bound is an instant"},
		{"bound subtracts from another function", "SELECT DISTINCT trace_id FROM spans WHERE start_time >= today() - INTERVAL '1' HOUR AND start_time < now()", "today()", "a start_time bound is an instant"},
		{"with fill", "SELECT * FROM spans WHERE " + window + " ORDER BY duration WITH FILL", "duration WITH", "WITH FILL is not supported"},
		{"order by unknown alias", "SELECT * FROM spans WHERE " + window + " ORDER BY t.duration", "t.duration", `unknown alias "t"`},
		{"order by twice", "SELECT * FROM spans WHERE " + window + " ORDER BY duration, duration DESC", "ORDER BY", "the order is not valid"},
		{"traces subquery with group by", "SELECT * FROM traces WHERE trace_id IN (SELECT trace_id FROM spans WHERE " + window + " GROUP BY service)", "GROUP BY", "GROUP BY is not supported yet"},
		{"traces subquery over an unknown relation", "SELECT * FROM traces WHERE trace_id IN (SELECT trace_id FROM logs WHERE " + window + ")", "logs", `unknown relation "logs"`},
		{"traces subquery without where", "SELECT * FROM traces WHERE trace_id IN (SELECT trace_id FROM spans)", "SELECT trace_id", "WHERE is required"},
		{"traces limit zero", "SELECT * FROM traces WHERE trace_id IN (SELECT trace_id FROM spans WHERE " + window + ") LIMIT 0", "0", `"0" is not a page size`},
		{"three-part column", spans + " AND a.b.c = 1", "a.b.c", "a column is written as alias.column or column"},
		{"subscript on a function", spans + " AND now()['x'] = 1", "now()['x']", "expected a column name here"},
		{"attr with a number", spans + " AND attr(1) = 'x'", "1) =", "the argument of attr() is a quoted attribute key"},
		{"subscript with an unknown alias", spans + " AND t.attributes['x'] = 1", "t.attributes", `unknown alias "t"`},
		{"subscript on the outer alias inside exists", spans + " AND EXISTS (SELECT 1 FROM events e WHERE e.span_id = s.span_id AND s.attributes['x'] = 1)", "s.attributes", "a predicate inside EXISTS reads the events columns only"},
		{"subscript with two keys", spans + " AND attributes['a', 'b'] = 1", "attributes['a'", "a map subscript names one attribute key"},
		{"interval month as a constant", spans + " AND name = INTERVAL '1' MONTH", "MONTH", `"MONTH" is not an interval unit`},
		{"interval of a function", spans + " AND name = INTERVAL now() HOUR", "INTERVAL now()", "an INTERVAL takes a number"},
		{"tuple as a constant", spans + " AND name = (1, 2)", "(1, 2)", "expected a literal here"},
		{"tuple as a predicate", spans + " AND (name = 'a', name = 'b')", "(name", "a parenthesized group holds one expression"},
		{"negated column as a predicate", spans + " AND -duration", "-duration", "the operator - is not supported"},
		{"not over an unsupported function", spans + " AND NOT lower(name) = 'x'", "lower(", "the function lower() is not supported"},
		{"string as a predicate", spans + " AND 'str'", "'str'", "expected a predicate here"},
		{"between with an unknown alias", spans + " AND t.duration BETWEEN '1s' AND '2s'", "t.duration", `unknown alias "t"`},
		{"between with a bad lower bound", spans + " AND duration BETWEEN lower(x) AND '2s'", "lower(x)", "expected a literal here"},
		{"between with a bad upper bound", spans + " AND duration BETWEEN '1s' AND lower(x)", "lower(x)", "expected a literal here"},
		{"is null with an unknown alias", spans + " AND t.attributes['x'] IS NULL", "t.attributes", `unknown alias "t"`},
		{"error on the left of or", spans + " AND (lower(name) = 'x' OR name = 'y')", "lower(", "the function lower() is not supported"},
		{"error on the right of or", spans + " AND (name = 'y' OR lower(name) = 'x')", "lower(", "the function lower() is not supported"},
		{"regexp without a string", spans + " AND name REGEXP duration", "duration", "the pattern of REGEXP is a quoted string"},
		{"comparison with a function constant", spans + " AND name = lower(x)", "lower(x)", "expected a literal here"},
		{"concatenation as a predicate", spans + " AND name || 'x'", "name ||", "the operator || is not supported"},
		{"in with a function", spans + " AND name IN (lower(x))", "lower(x)", "expected a literal here"},
		{"exists with two arguments", spans + " AND EXISTS(1, 2)", "EXISTS", "EXISTS takes one subquery"},
		{"exists with a number", spans + " AND EXISTS(1)", "1)", "EXISTS takes a subquery"},
		{"match with one argument", spans + " AND match(name)", "match(", "match() takes a column and a quoted pattern"},
		{"match with an unknown alias", spans + " AND match(t.name, 'x')", "t.name", `unknown alias "t"`},
		{"phrase with a number", spans + " AND phrase(attributes['x'], 1)", "1)", "the pattern of phrase() is a quoted string"},
		{"match with two patterns", spans + " AND match(name, 'a', 'b')", "match(", "match() takes a column and one pattern"},
		{"function as a predicate", spans + " AND lower(name)", "lower(", "the function lower() is not supported"},
		{"exists with group by", spans + " AND EXISTS (SELECT 1 FROM events e WHERE e.span_id = s.span_id AND e.name = 'x' GROUP BY e.name)", "GROUP BY", "GROUP BY is not supported yet"},
		{"exists over an unknown relation", spans + " AND EXISTS (SELECT 1 FROM logs l WHERE l.span_id = s.span_id)", "logs", `unknown relation "logs"`},
		{"correlation with an unknown inner column", spans + " AND EXISTS (SELECT 1 FROM events e WHERE e.foo = s.span_id)", "e.foo", `events has no column "foo"`},
		{"correlation with an unknown outer column", spans + " AND EXISTS (SELECT 1 FROM links l WHERE l.span_id = s.foo)", "l.span_id", `"span_id" is a correlation column, not a predicate`},
		{"links unknown column", spans + " AND EXISTS (SELECT 1 FROM links l WHERE l.span_id = s.span_id AND l.foo = 1)", "l.foo", `links has no column "foo"`},
		{"event column compared with the outer span", spans + " AND EXISTS (SELECT 1 FROM events e WHERE e.span_id = s.span_id AND e.name = s.name)", "s.name", "a predicate inside EXISTS reads the events columns only"},
		{"traces aggregate", "SELECT count(*) FROM traces WHERE trace_id IN (SELECT trace_id FROM spans WHERE " + window + ")", "count", "count() is an aggregate"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Compile(tt.sql, now)
			require.Error(t, err)
			var refusal *Error
			require.ErrorAs(t, err, &refusal)
			assert.Contains(t, err.Error(), tt.want)
			assert.Equal(t, 1, refusal.Line)
			if tt.at == "" {
				assert.Equal(t, len(tt.sql)+1, refusal.Column, err.Error())
				return
			}
			assert.Equal(t, strings.Index(tt.sql, tt.at)+1, refusal.Column, err.Error())
		})
	}
}

func TestErrorRendering(t *testing.T) {
	err := &Error{Line: 2, Column: 7, Message: "the operator ^ is not supported"}
	assert.Equal(t, "line 2:7: the operator ^ is not supported", err.Error())
	err.Hint = "Use REGEXP"
	assert.Equal(t, "line 2:7: the operator ^ is not supported. Use REGEXP", err.Error())
}

func TestPositionsSpanLines(t *testing.T) {
	_, err := Compile("SELECT DISTINCT trace_id\nFROM spans\nWHERE "+window+"\n  AND lower(name) = 'x'", now)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "line 4:7: the function lower() is not supported")
}

func TestLikeToRegex(t *testing.T) {
	assert.Equal(t, `a\.b.*c.`, likeToRegex("a.b%c_"))
	assert.Equal(t, `\(x\)`, likeToRegex("(x)"))
}

func TestUnquote(t *testing.T) {
	assert.Equal(t, "o'brien", unquote("o''brien"))
	assert.Equal(t, `a'b\c`, unquote(`a\'b\\c`))
	assert.Equal(t, `trailing\`, unquote(`trailing\`))
}
