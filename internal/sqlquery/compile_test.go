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

// Every statement in this file is laid out the same way, so that a reader scans the corpus as
// SQL rather than as Go strings: one clause per line (SELECT, FROM, WHERE, ORDER BY, LIMIT), each
// AND or OR on its own line indented one level under WHERE, a subquery indented one level inside
// its parentheses with the closing parenthesis on its own line, and the expected tree with one
// predicate per line. The layout is a convention reviewers hold the file to. The parser library
// has a beautifying formatter, but its layout differs from this one, so a Go formatter that
// could enforce the convention from a test is left to the UI SQL mode of RFC 0019 §9, which
// needs one for its format button.

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
			sql: `
				SELECT DISTINCT trace_id
				FROM spans
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()`,
			want: nil,
		},
		{
			name: "service and attribute",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND s.service = 'checkout'
					AND s.attributes['http.status_code'] >= 500`,
			want: p.And(
				p.Resource().Service.Eq("checkout"),
				p.Span().Attr("http.status_code").Gte("500"),
			),
		},
		{
			name: "camelCase columns and the IDL spelling of a field",
			sql: `
				SELECT DISTINCT traceID
				FROM spans
				WHERE startTime >= now() - INTERVAL '1' HOUR
					AND startTime < now()
					AND parentSpanId = ''
					AND statusMessage <> 'ok'`,
			want: p.And(
				p.Span().ParentSpanID.Eq(""),
				p.Span().StatusMessage.Ne("ok"),
			),
		},
		{
			name: "every comparison operator",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND duration > '2s'
					AND duration < '10s'
					AND duration >= '1ms'
					AND duration <= '1us'
					AND name != 'a'
					AND name == 'b'`,
			want: p.And(
				p.Span().Duration.Gt("2s"),
				p.Span().Duration.Lt("10s"),
				p.Span().Duration.Gte("1ms"),
				p.Span().Duration.Lte("1us"),
				p.Span().Name.Ne("a"),
				p.Span().Name.Eq("b"),
			),
		},
		{
			name: "a constant on the left is mirrored",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND '2s' < duration
					AND 'x' = name`,
			want: p.And(
				p.Span().Duration.Gt("2s"),
				p.Span().Name.Eq("x"),
			),
		},
		{
			name: "interval as a duration constant",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND duration > INTERVAL '250' MILLISECOND
					AND duration < INTERVAL 2 SECOND`,
			want: p.And(
				p.Span().Duration.Gt("250ms"),
				p.Span().Duration.Lt("2s"),
			),
		},
		{
			name: "or, not and parentheses",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND (kind = 'server' OR kind = 'client')
					AND NOT status = 'ok'`,
			want: p.And(
				p.Or(
					p.Span().Kind.Eq("server"),
					p.Span().Kind.Eq("client"),
				),
				p.Not(p.Span().Status.Eq("ok")),
			),
		},
		{
			name: "in and not in",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND kind IN ('server', 'consumer')
					AND attributes['http.method'] NOT IN ('GET', 'HEAD')`,
			want: p.And(
				p.Span().Kind.In("server", "consumer"),
				p.Span().Attr("http.method").NotIn("GET", "HEAD"),
			),
		},
		{
			name: "like and not like",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND name LIKE '%/api/%'
					AND name NOT LIKE '%health_check%'
					AND status_message LIKE 'exact (text)'`,
			want: p.And(
				p.Span().Name.Matches(`/api/`),
				p.Not(p.Span().Name.Matches(`health.check`)),
				p.Span().StatusMessage.Eq("exact (text)"),
			),
		},
		{
			name: "regexp and match",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND name REGEXP 'GET|POST'
					AND match(attributes['http.route'], '/users/[0-9]+')`,
			want: p.And(
				p.Span().Name.Matches("GET|POST"),
				p.Span().Attr("http.route").Matches("/users/[0-9]+"),
			),
		},
		{
			name: "is null and is not null on attributes",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND attributes['error'] IS NOT NULL
					AND resource_attributes['k8s.pod.name'] IS NULL`,
			want: p.And(
				p.Span().Attr("error").Exists(),
				p.Not(p.Resource().Attr("k8s.pod.name").Exists()),
			),
		},
		{
			name: "phrase and fulltext",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND phrase(attributes['db.statement'], 'connection refused')
					AND fulltext(attributes['exception.message'], 'timeout', 'upstream')`,
			want: p.And(
				p.Span().Attr("db.statement").Phrase("connection", "refused"),
				p.Span().Attr("exception.message").Fulltext("timeout", "upstream"),
			),
		},
		{
			name: "between",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND duration BETWEEN '1s' AND '2s'
					AND attributes['retries'] NOT BETWEEN 1 AND 3`,
			want: p.And(
				p.Span().Duration.Gte("1s"),
				p.Span().Duration.Lte("2s"),
				p.Not(
					p.And(
						p.Span().Attr("retries").Gte("1"),
						p.Span().Attr("retries").Lte("3"),
					),
				),
			),
		},
		{
			name: "unqualified attribute, boolean and numbers",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND attr('error') = true
					AND attr('retries') > 3
					AND attr('ratio') <= 0.5`,
			want: p.And(
				p.Attr("error").Eq("true"),
				p.Attr("retries").Gt("3"),
				p.Attr("ratio").Lte("0.5"),
			),
		},
		{
			name: "two columns compared",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND start_time < end_time
					AND attributes['after'] > attributes['before']`,
			want: p.And(
				p.Span().StartTime.Lt(p.Span().EndTime),
				p.Span().Attr("after").Gt(p.Span().Attr("before")),
			),
		},
		{
			name: "a second start_time predicate stays in the filter",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND (start_time < '2026-10-23T11:30:00Z' OR duration > '1s')`,
			want: p.Or(
				p.Span().StartTime.Lt("2026-10-23T11:30:00Z"),
				p.Span().Duration.Gt("1s"),
			),
		},
		{
			name: "scope columns",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND scope_name = 'io.opentelemetry.http'
					AND scope_version LIKE '%1.%'
					AND scope_attributes['x'] = 'y'`,
			want: p.And(
				p.Scope().Name.Eq("io.opentelemetry.http"),
				p.Scope().Version.Matches(`1\.`),
				p.Scope().Attr("x").Eq("y"),
			),
		},
		{
			name: "exists over events",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND s.service = 'checkout'
					AND EXISTS (
						SELECT 1
						FROM events e
						WHERE e.span_id = s.span_id
							AND e.trace_id = s.trace_id
							AND e.name = 'exception'
							AND e.attributes['exception.type'] = 'TimeoutError'
					)`,
			want: p.And(
				p.Resource().Service.Eq("checkout"),
				p.Some(
					p.Event(),
					p.And(
						p.Event().Name.Eq("exception"),
						p.Event().Attr("exception.type").Eq("TimeoutError"),
					),
				),
			),
		},
		{
			name: "exists over links, correlation last and reversed",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND EXISTS (
						SELECT 1
						FROM links l
						WHERE l.linked_trace_id = 'abc'
							AND l.attributes['time_since'] IS NULL
							AND s.span_id = l.span_id
					)`,
			want: p.Some(
				p.Link(),
				p.And(
					p.Link().TraceID.Eq("abc"),
					p.Not(p.Link().Attr("time_since").Exists()),
				),
			),
		},
		{
			name: "not exists",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND NOT EXISTS (
						SELECT 1
						FROM events e
						WHERE e.span_id = s.span_id
							AND e.name = 'retry'
					)`,
			want: p.Not(
				p.Some(
					p.Event(),
					p.Event().Name.Eq("retry"),
				),
			),
		},
		{
			name: "nested boolean structure",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND NOT (kind = 'server' OR (status = 'error' AND duration > '1s'))
					AND ( (name = 'a' AND name = 'b') OR name = 'c')`,
			want: p.And(
				p.Not(
					p.Or(
						p.Span().Kind.Eq("server"),
						p.And(
							p.Span().Status.Eq("error"),
							p.Span().Duration.Gt("1s"),
						),
					),
				),
				p.Or(
					p.And(
						p.Span().Name.Eq("a"),
						p.Span().Name.Eq("b"),
					),
					p.Span().Name.Eq("c"),
				),
			),
		},
		{
			name: "start_time predicates that are not bounds stay in the filter",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND start_time <> '2026-10-23T11:30:00Z'
					AND attributes['deadline'] > now() + INTERVAL '30' SECOND`,
			want: p.And(
				p.Span().StartTime.Ne("2026-10-23T11:30:00Z"),
				p.Span().Attr("deadline").Gt("2026-10-23T12:00:30Z"),
			),
		},
		{
			name: "quoted quote in a literal",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND name = 'o''brien'
					AND status_message = 'a\'b'`,
			want: p.And(
				p.Span().Name.Eq("o'brien"),
				p.Span().StatusMessage.Eq("a'b"),
			),
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
			sql: `
				SELECT DISTINCT trace_id
				FROM spans
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND service = 'checkout'
				LIMIT 20`,
			want: Query{Kind: KindFindTraceIDs, Filter: spanFilter, Limit: 20},
		},
		{
			name: "star is FindSpans",
			sql: `
				SELECT *
				FROM spans
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND service = 'checkout'
				ORDER BY duration DESC, start_time
				LIMIT 100`,
			want: Query{Kind: KindFindSpans, Filter: spanFilter, Limit: 100, OrderBy: []tracestore.SpanSortOrder{
				{Expression: &ast.FieldRef{Name: ast.SpanFieldDuration, Level: ast.LevelSpan}, Direction: tracestore.SortDescending},
				{Expression: &ast.FieldRef{Name: ast.SpanFieldStartTime, Level: ast.LevelSpan}, Direction: tracestore.SortAscending},
			}},
		},
		{
			name: "column list is FindSpans with a projection",
			sql: `
				SELECT s.span_id, duration, attributes['http.route']
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
				ORDER BY s.trace_id, s.span_id`,
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
			sql: `
				SELECT *
				FROM traces
				WHERE trace_id IN (
					SELECT trace_id
					FROM spans s
					WHERE start_time >= now() - INTERVAL '1' HOUR
						AND start_time < now()
						AND s.service = 'checkout'
				)
				LIMIT 5`,
			want: Query{Kind: KindFindTraces, Filter: spanFilter, Limit: 5},
		},
		{
			name: "traces columns is FindTraceSummaries",
			sql: `
				SELECT t.trace_id, root_service, span_count, error_count
				FROM traces t
				WHERE t.trace_id IN (
					SELECT DISTINCT trace_id
					FROM spans
					WHERE start_time >= now() - INTERVAL '1' HOUR
						AND start_time < now()
				)`,
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
	tests := []struct {
		name string
		sql  string
		at   string // the text the refusal points at, or "" when the error is at the end of the statement
		want string
	}{
		{
			name: "syntax error",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans
				WHERE start_time >= 'a'
					AND`,
			at:   "",
			want: "The statement did not parse as SQL",
		},
		{
			name: "tilde is not an operator",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND name ~ 'x'`,
			at:   "~",
			want: "did not parse",
		},
		{
			name: "timestamp keyword",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND start_time >= TIMESTAMP '2026-01-01T00:00:00Z'`,
			at:   "TIMESTAMP",
			want: "did not parse",
		},
		{
			name: "two statements",
			sql: `
				SELECT 1; SELECT 2`,
			at:   "SELECT 2",
			want: "the request holds 2 statements",
		},
		{
			name: "not a select",
			sql: `
				INSERT INTO spans VALUES(1)`,
			at:   "INSERT",
			want: "only a SELECT statement is accepted",
		},
		{
			name: "with",
			sql: `
				WITH x AS (
					SELECT 1
				) SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()`,
			at:   "WITH",
			want: "WITH is not supported",
		},
		{
			name: "union",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
				UNION ALL SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()`,
			at:   "SELECT",
			want: "UNION, EXCEPT and INTERSECT are not supported",
		},
		{
			name: "group by",
			sql: `
				SELECT service
				FROM spans
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
				GROUP BY service`,
			at:   "GROUP BY",
			want: "GROUP BY is not supported yet",
		},
		{
			name: "having",
			sql: `
				SELECT service
				FROM spans
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
				GROUP BY service
				HAVING count() > 1`,
			at:   "GROUP BY",
			want: "GROUP BY is not supported yet",
		},
		{
			name: "aggregate in select",
			sql: `
				SELECT count(*)
				FROM spans
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()`,
			at:   "count",
			want: "count() is an aggregate",
		},
		{
			name: "aggregate in where",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND count() > 1`,
			at:   "count",
			want: "count() is an aggregate",
		},
		{
			name: "offset",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
				LIMIT 10 OFFSET 5`,
			at:   "5",
			want: "OFFSET is not supported",
		},
		{
			name: "limit zero",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
				LIMIT 0`,
			at:   "0",
			want: `"0" is not a page size`,
		},
		{
			name: "limit not a number",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
				LIMIT 'ten'`,
			at:   "'ten'",
			want: "LIMIT takes a whole number",
		},
		{
			name: "settings",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
				SETTINGS max_threads = 1`,
			at:   "SETTINGS",
			want: "SETTINGS is not supported",
		},
		{
			name: "format",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
				FORMAT JSON`,
			at:   "FORMAT",
			want: "FORMAT is not supported",
		},
		{
			name: "no from",
			sql: `
				SELECT 1
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()`,
			at:   "SELECT",
			want: "FROM is required",
		},
		{
			name: "join",
			sql: `
				SELECT DISTINCT a.trace_id
				FROM spans a JOIN spans b ON a.trace_id = b.trace_id
				WHERE a.start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()`,
			at:   "spans a JOIN",
			want: "JOIN is not supported",
		},
		{
			name: "derived table",
			sql: `
				SELECT DISTINCT trace_id
				FROM (
					SELECT *
					FROM spans
				)
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()`,
			at:   "SELECT *",
			want: "FROM takes a relation name",
		},
		{
			name: "unknown relation",
			sql: `
				SELECT DISTINCT trace_id
				FROM logs
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()`,
			at:   "logs",
			want: `unknown relation "logs"`,
		},
		{
			name: "events in from",
			sql: `
				SELECT DISTINCT trace_id
				FROM events
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()`,
			at:   "FROM events",
			want: "events is read through EXISTS",
		},
		{
			name: "database qualifier",
			sql: `
				SELECT DISTINCT trace_id
				FROM jaeger.spans
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()`,
			at:   "jaeger.spans",
			want: "a relation has no database qualifier",
		},
		{
			name: "distinct other column",
			sql: `
				SELECT DISTINCT name
				FROM spans
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()`,
			at:   "SELECT",
			want: "DISTINCT is accepted only as SELECT DISTINCT trace_id",
		},
		{
			name: "expression in select",
			sql: `
				SELECT duration * 2
				FROM spans
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()`,
			at:   "duration * 2",
			want: "SELECT lists columns, or *",
		},
		{
			name: "alias in select",
			sql: `
				SELECT duration AS d
				FROM spans
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()`,
			at:   "d\n",
			want: "a column alias in SELECT has no effect",
		},
		{
			name: "no where",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans`,
			at:   "SELECT",
			want: "WHERE is required",
		},
		{
			name: "no window",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans
				WHERE service = 'a'`,
			at:   "WHERE",
			want: "WHERE needs both bounds on start_time",
		},
		{
			name: "no upper bound",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans
				WHERE start_time >= now() - INTERVAL '1' HOUR`,
			at:   "WHERE",
			want: "WHERE needs an upper bound on start_time",
		},
		{
			name: "no lower bound",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans
				WHERE start_time < now()`,
			at:   "WHERE",
			want: "WHERE needs a lower bound on start_time",
		},
		{
			name: "empty window",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans
				WHERE start_time >= now()
					AND start_time < now() - INTERVAL '1' HOUR`,
			at:   "WHERE",
			want: "the start_time window is empty",
		},
		{
			name: "bound twice",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND s.start_time >= now() - INTERVAL '2' HOUR`,
			at:   "s.start_time >= now() - INTERVAL '2'",
			want: "start_time is bounded twice on the same side",
		},
		{
			name: "bound not an instant",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans
				WHERE start_time >= 'yesterday'
					AND start_time < now()`,
			at:   "'yesterday'",
			want: `"yesterday" is not an RFC 3339 instant`,
		},
		{
			name: "bound is a number",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans
				WHERE start_time >= 1700000000
					AND start_time < now()`,
			at:   "1700000000",
			want: "a start_time bound is an instant",
		},
		{
			name: "now with arguments",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans
				WHERE start_time >= now('UTC')
					AND start_time < now()`,
			at:   "now('UTC')",
			want: "now() takes no arguments",
		},
		{
			name: "now minus a number",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans
				WHERE start_time >= now() - 3600
					AND start_time < now()`,
			at:   "3600",
			want: "only an INTERVAL may be added to or subtracted from now()",
		},
		{
			name: "interval month",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans
				WHERE start_time >= now() - INTERVAL '1' MONTH
					AND start_time < now()`,
			at:   "MONTH",
			want: `"MONTH" is not an interval unit the dialect knows`,
		},
		{
			name: "interval fraction",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans
				WHERE start_time >= now() - INTERVAL '1.5' HOUR
					AND start_time < now()`,
			at:   "INTERVAL '1.5'",
			want: `"1.5" is not a whole number`,
		},
		{
			name: "unknown column",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND s.operation = 'x'`,
			at:   "s.operation",
			want: `spans has no column "operation"`,
		},
		{
			name: "unknown alias",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND t.name = 'x'`,
			at:   "t.name",
			want: `unknown alias "t"`,
		},
		{
			name: "map without subscript",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND attributes = 'x'`,
			at:   "attributes =",
			want: `"attributes" is a map and is read by subscript`,
		},
		{
			name: "subscript on a field",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND name['x'] = 'y'`,
			at:   "name['x']",
			want: `"name" is not a map column and takes no subscript`,
		},
		{
			name: "subscript not a string",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND attributes[1] = 'y'`,
			at:   "[1]",
			want: "the subscript of a map column is a quoted attribute key",
		},
		{
			name: "attr with two arguments",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND attr('a', 'b') = 'y'`,
			at:   "attr('a'",
			want: "attr() takes one argument",
		},
		{
			name: "is null on a field",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND name IS NULL`,
			at:   "name IS NULL",
			want: "IS NULL applies to an attribute",
		},
		{
			name: "duration without unit",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND duration > 2000`,
			at:   "WHERE",
			want: "the filter is not valid",
		},
		{
			name: "ordered operator on kind",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND kind > 'server'`,
			at:   "WHERE",
			want: "the filter is not valid",
		},
		{
			name: "two constants",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND 1 = 1`,
			at:   "1 = 1",
			want: "a comparison names a column on at least one side",
		},
		{
			name: "in with a subquery",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND trace_id IN (
						SELECT trace_id
						FROM spans
					)`,
			at:   "SELECT trace_id",
			want: "a subquery in IN is accepted only as",
		},
		{
			name: "in without a list",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND name IN 'a'`,
			at:   "'a'",
			want: "IN takes a parenthesized list of literals",
		},
		{
			name: "like anchored at one end",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND name LIKE 'GET%'`,
			at:   "'GET%'",
			want: `LIKE "GET%" is anchored at one end`,
		},
		{
			name: "like anchored at the other end",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND name LIKE '%GET'`,
			at:   "'%GET'",
			want: `LIKE "%GET" is anchored at one end`,
		},
		{
			name: "ilike",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND name ILIKE '%get%'`,
			at:   "name ILIKE",
			want: "ILIKE is not supported",
		},
		{
			name: "exists selects star",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND EXISTS (
						SELECT *
						FROM events e
						WHERE e.span_id = s.span_id
							AND e.name = 'x'
					)`,
			at:   "FROM events",
			want: "did not parse",
		},
		{
			name: "like without a string",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND name LIKE duration`,
			at:   "duration",
			want: "the pattern of LIKE is a quoted string",
		},
		{
			name: "unsupported function",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND lower(name) = 'x'`,
			at:   "lower(",
			want: "the function lower() is not supported",
		},
		{
			name: "arithmetic",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND duration * 2 > '1s'`,
			at:   "duration * 2",
			want: "the operator * is not supported",
		},
		{
			name: "unary minus",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND -duration > '1s'`,
			at:   "-duration",
			want: "the operator - is not supported",
		},
		{
			name: "exists without correlation",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND EXISTS (
						SELECT 1
						FROM events e
						WHERE e.name = 'x'
					)`,
			at:   "WHERE e.name",
			want: "an EXISTS subquery is tied to its span by e.span_id = s.span_id",
		},
		{
			name: "exists without a predicate",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND EXISTS (
						SELECT 1
						FROM events e
						WHERE e.span_id = s.span_id
					)`,
			at:   "WHERE e.span_id",
			want: "an EXISTS subquery needs a predicate on events beside the correlation",
		},
		{
			name: "exists without where",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND EXISTS (
						SELECT 1
						FROM events e
					)`,
			at:   "SELECT 1",
			want: "an EXISTS subquery needs WHERE e.span_id = s.span_id",
		},
		{
			name: "exists over spans",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND EXISTS (
						SELECT 1
						FROM spans x
						WHERE x.span_id = s.span_id
					)`,
			at:   "FROM spans x",
			want: "EXISTS reads events or links",
		},
		{
			name: "exists reads the outer alias",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND EXISTS (
						SELECT 1
						FROM events e
						WHERE e.span_id = s.span_id
							AND s.name = 'x'
					)`,
			at:   "s.name = 'x'",
			want: "a predicate inside EXISTS reads the events columns only",
		},
		{
			name: "exists with a limit",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND EXISTS (
						SELECT 1
						FROM events e
						WHERE e.span_id = s.span_id
							AND e.name = 'x'
						LIMIT 1
					)`,
			at:   "SELECT 1",
			want: "an EXISTS subquery takes no ORDER BY or LIMIT",
		},
		{
			name: "correlation on different columns",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND EXISTS (
						SELECT 1
						FROM events e
						WHERE e.span_id = s.trace_id
							AND e.name = 'x'
					)`,
			at:   "e.span_id = s.trace_id",
			want: "a correlation ties the same column",
		},
		{
			name: "correlation column as a predicate",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND EXISTS (
						SELECT 1
						FROM events e
						WHERE e.span_id = s.span_id
							AND e.trace_id = 'abc'
					)`,
			at:   "e.trace_id = 'abc'",
			want: `"trace_id" is a correlation column, not a predicate`,
		},
		{
			name: "nested exists",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND EXISTS (
						SELECT 1
						FROM events e
						WHERE e.span_id = s.span_id
							AND EXISTS (
								SELECT 1
								FROM links l
								WHERE l.span_id = e.span_id
									AND l.trace_state = 'x'
							)
					)`,
			at:   "FROM links",
			want: "EXISTS appears in the WHERE of a spans query",
		},
		{
			name: "order by on trace ids",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
				ORDER BY duration`,
			at:   "ORDER BY",
			want: "ORDER BY applies to a span search",
		},
		{
			name: "order by unsortable",
			sql: `
				SELECT *
				FROM spans
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
				ORDER BY name`,
			at:   "name",
			want: "ORDER BY takes a sortable column",
		},
		{
			name: "order by attribute",
			sql: `
				SELECT *
				FROM spans
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
				ORDER BY attributes['x']`,
			at:   "attributes['x']",
			want: "ORDER BY takes a sortable column",
		},
		{
			name: "traces without where",
			sql: `
				SELECT *
				FROM traces`,
			at:   "SELECT",
			want: "a traces query is SELECT * FROM traces WHERE trace_id IN",
		},
		{
			name: "traces with a predicate",
			sql: `
				SELECT *
				FROM traces
				WHERE duration > '5s'`,
			at:   "WHERE",
			want: "the WHERE of a traces query is trace_id IN",
		},
		{
			name: "traces in without subquery",
			sql: `
				SELECT *
				FROM traces
				WHERE trace_id IN ('a', 'b')`,
			at:   "('a'",
			want: "trace_id IN takes the subquery",
		},
		{
			name: "traces subquery over events",
			sql: `
				SELECT *
				FROM traces
				WHERE trace_id IN (
					SELECT trace_id
					FROM events
					WHERE start_time >= now() - INTERVAL '1' HOUR
						AND start_time < now()
				)`,
			at:   "FROM events",
			want: "the subquery of a traces query reads spans",
		},
		{
			name: "traces subquery selects a span column",
			sql: `
				SELECT *
				FROM traces
				WHERE trace_id IN (
					SELECT span_id
					FROM spans
					WHERE start_time >= now() - INTERVAL '1' HOUR
						AND start_time < now()
				)`,
			at:   "SELECT span_id",
			want: "the subquery of a traces query selects trace_id",
		},
		{
			name: "traces subquery with limit",
			sql: `
				SELECT *
				FROM traces
				WHERE trace_id IN (
					SELECT trace_id
					FROM spans
					WHERE start_time >= now() - INTERVAL '1' HOUR
						AND start_time < now()
					LIMIT 5
				)`,
			at:   "SELECT trace_id",
			want: "ORDER BY and LIMIT go on the outer traces query",
		},
		{
			name: "traces order by",
			sql: `
				SELECT *
				FROM traces
				WHERE trace_id IN (
					SELECT trace_id
					FROM spans
					WHERE start_time >= now() - INTERVAL '1' HOUR
						AND start_time < now()
				)
				ORDER BY duration`,
			at:   "ORDER BY",
			want: "a trace search has no ORDER BY",
		},
		{
			name: "traces unknown column",
			sql: `
				SELECT trace_id, name
				FROM traces
				WHERE trace_id IN (
					SELECT trace_id
					FROM spans
					WHERE start_time >= now() - INTERVAL '1' HOUR
						AND start_time < now()
				)`,
			at:   "name",
			want: `traces has no column "name"`,
		},
		{
			name: "parenthesized query",
			sql: `
				(
					SELECT DISTINCT trace_id
					FROM spans s
					WHERE start_time >= now() - INTERVAL '1' HOUR
						AND start_time < now()
				)`,
			at:   "(",
			want: "a parenthesized query is not supported",
		},
		{
			name: "having without group by",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
				HAVING count() > 1`,
			at:   "HAVING",
			want: "HAVING is not supported yet",
		},
		{
			name: "window clause",
			sql: `
				SELECT *
				FROM spans
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
				WINDOW w AS(PARTITION BY service)`,
			at:   "WINDOW",
			want: "window functions are not supported",
		},
		{
			name: "prewhere",
			sql: `
				SELECT *
				FROM spans
				PREWHERE service = 'a'
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()`,
			at:   "PREWHERE",
			want: "PREWHERE is not supported",
		},
		{
			name: "limit by",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
				LIMIT 1 BY service`,
			at:   "LIMIT 1 BY",
			want: "LIMIT BY is not supported",
		},
		{
			name: "top",
			sql: `
				SELECT TOP 5 *
				FROM spans
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()`,
			at:   "TOP",
			want: "TOP is not supported",
		},
		{
			name: "distinct on",
			sql: `
				SELECT DISTINCT ON (trace_id) *
				FROM spans
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()`,
			at:   "ON",
			want: "DISTINCT ON is not supported",
		},
		{
			name: "table function",
			sql: `
				SELECT *
				FROM numbers(10)
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()`,
			at:   "numbers",
			want: "FROM takes a relation name",
		},
		{
			name: "unknown projected column",
			sql: `
				SELECT nonexistent
				FROM spans
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()`,
			at:   "nonexistent",
			want: `spans has no column "nonexistent"`,
		},
		{
			name: "between window with a bad lower bound",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans
				WHERE start_time BETWEEN 'x' AND now()`,
			at:   "'x'",
			want: `"x" is not an RFC 3339 instant`,
		},
		{
			name: "between window with a bad upper bound",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans
				WHERE start_time BETWEEN now() AND 'y'`,
			at:   "'y'",
			want: `"y" is not an RFC 3339 instant`,
		},
		{
			name: "between window beside a bound",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND start_time BETWEEN now() - INTERVAL '2' HOUR AND now()`,
			at:   "start_time BETWEEN",
			want: "start_time is bounded twice",
		},
		{
			name: "bound is another function",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans
				WHERE start_time >= today()
					AND start_time < now()`,
			at:   "today()",
			want: "a start_time bound is an instant",
		},
		{
			name: "bound is now times two",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans
				WHERE start_time >= now() * 2
					AND start_time < now()`,
			at:   "now() * 2",
			want: "a start_time bound is an instant",
		},
		{
			name: "bound subtracts from another function",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans
				WHERE start_time >= today() - INTERVAL '1' HOUR
					AND start_time < now()`,
			at:   "today()",
			want: "a start_time bound is an instant",
		},
		{
			name: "with fill",
			sql: `
				SELECT *
				FROM spans
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
				ORDER BY duration WITH FILL`,
			at:   "duration WITH",
			want: "WITH FILL is not supported",
		},
		{
			name: "order by unknown alias",
			sql: `
				SELECT *
				FROM spans
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
				ORDER BY t.duration`,
			at:   "t.duration",
			want: `unknown alias "t"`,
		},
		{
			name: "order by twice",
			sql: `
				SELECT *
				FROM spans
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
				ORDER BY duration, duration DESC`,
			at:   "ORDER BY",
			want: "the order is not valid",
		},
		{
			name: "traces subquery with group by",
			sql: `
				SELECT *
				FROM traces
				WHERE trace_id IN (
					SELECT trace_id
					FROM spans
					WHERE start_time >= now() - INTERVAL '1' HOUR
						AND start_time < now()
					GROUP BY service
				)`,
			at:   "GROUP BY",
			want: "GROUP BY is not supported yet",
		},
		{
			name: "traces subquery over an unknown relation",
			sql: `
				SELECT *
				FROM traces
				WHERE trace_id IN (
					SELECT trace_id
					FROM logs
					WHERE start_time >= now() - INTERVAL '1' HOUR
						AND start_time < now()
				)`,
			at:   "logs",
			want: `unknown relation "logs"`,
		},
		{
			name: "traces subquery without where",
			sql: `
				SELECT *
				FROM traces
				WHERE trace_id IN (
					SELECT trace_id
					FROM spans
				)`,
			at:   "SELECT trace_id",
			want: "WHERE is required",
		},
		{
			name: "traces limit zero",
			sql: `
				SELECT *
				FROM traces
				WHERE trace_id IN (
					SELECT trace_id
					FROM spans
					WHERE start_time >= now() - INTERVAL '1' HOUR
						AND start_time < now()
				)
				LIMIT 0`,
			at:   "0",
			want: `"0" is not a page size`,
		},
		{
			name: "three-part column",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND a.b.c = 1`,
			at:   "a.b.c",
			want: "a column is written as alias.column or column",
		},
		{
			name: "subscript on a function",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND now()['x'] = 1`,
			at:   "now()['x']",
			want: "expected a column name here",
		},
		{
			name: "attr with a number",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND attr(1) = 'x'`,
			at:   "1) =",
			want: "the argument of attr() is a quoted attribute key",
		},
		{
			name: "subscript with an unknown alias",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND t.attributes['x'] = 1`,
			at:   "t.attributes",
			want: `unknown alias "t"`,
		},
		{
			name: "subscript on the outer alias inside exists",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND EXISTS (
						SELECT 1
						FROM events e
						WHERE e.span_id = s.span_id
							AND s.attributes['x'] = 1
					)`,
			at:   "s.attributes",
			want: "a predicate inside EXISTS reads the events columns only",
		},
		{
			name: "subscript with two keys",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND attributes['a', 'b'] = 1`,
			at:   "attributes['a'",
			want: "a map subscript names one attribute key",
		},
		{
			name: "interval month as a constant",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND name = INTERVAL '1' MONTH`,
			at:   "MONTH",
			want: `"MONTH" is not an interval unit`,
		},
		{
			name: "interval of a function",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND name = INTERVAL now() HOUR`,
			at:   "INTERVAL now()",
			want: "an INTERVAL takes a number",
		},
		{
			name: "tuple as a constant",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND name = (1, 2)`,
			at:   "(1, 2)",
			want: "expected a literal here",
		},
		{
			name: "tuple as a predicate",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND (name = 'a', name = 'b')`,
			at:   "(name",
			want: "a parenthesized group holds one expression",
		},
		{
			name: "negated column as a predicate",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND -duration`,
			at:   "-duration",
			want: "the operator - is not supported",
		},
		{
			name: "not over an unsupported function",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND NOT lower(name) = 'x'`,
			at:   "lower(",
			want: "the function lower() is not supported",
		},
		{
			name: "string as a predicate",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND 'str'`,
			at:   "'str'",
			want: "expected a predicate here",
		},
		{
			name: "between with an unknown alias",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND t.duration BETWEEN '1s' AND '2s'`,
			at:   "t.duration",
			want: `unknown alias "t"`,
		},
		{
			name: "between with a bad lower bound",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND duration BETWEEN lower(x) AND '2s'`,
			at:   "lower(x)",
			want: "expected a literal here",
		},
		{
			name: "between with a bad upper bound",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND duration BETWEEN '1s' AND lower(x)`,
			at:   "lower(x)",
			want: "expected a literal here",
		},
		{
			name: "is null with an unknown alias",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND t.attributes['x'] IS NULL`,
			at:   "t.attributes",
			want: `unknown alias "t"`,
		},
		{
			name: "error on the left of or",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND (lower(name) = 'x' OR name = 'y')`,
			at:   "lower(",
			want: "the function lower() is not supported",
		},
		{
			name: "error on the right of or",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND (name = 'y' OR lower(name) = 'x')`,
			at:   "lower(",
			want: "the function lower() is not supported",
		},
		{
			name: "regexp without a string",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND name REGEXP duration`,
			at:   "duration",
			want: "the pattern of REGEXP is a quoted string",
		},
		{
			name: "comparison with a function constant",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND name = lower(x)`,
			at:   "lower(x)",
			want: "expected a literal here",
		},
		{
			name: "concatenation as a predicate",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND name || 'x'`,
			at:   "name ||",
			want: "the operator || is not supported",
		},
		{
			name: "in with a function",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND name IN (lower(x))`,
			at:   "lower(x)",
			want: "expected a literal here",
		},
		{
			name: "exists with two arguments",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND EXISTS(1, 2)`,
			at:   "EXISTS",
			want: "EXISTS takes one subquery",
		},
		{
			name: "exists with a number",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND EXISTS(1)`,
			at:   "1)",
			want: "EXISTS takes a subquery",
		},
		{
			name: "match with one argument",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND match(name)`,
			at:   "match(",
			want: "match() takes a column and a quoted pattern",
		},
		{
			name: "match with an unknown alias",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND match(t.name, 'x')`,
			at:   "t.name",
			want: `unknown alias "t"`,
		},
		{
			name: "phrase with a number",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND phrase(attributes['x'], 1)`,
			at:   "1)",
			want: "the pattern of phrase() is a quoted string",
		},
		{
			name: "match with two patterns",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND match(name, 'a', 'b')`,
			at:   "match(",
			want: "match() takes a column and one pattern",
		},
		{
			name: "function as a predicate",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND lower(name)`,
			at:   "lower(",
			want: "the function lower() is not supported",
		},
		{
			name: "exists with group by",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND EXISTS (
						SELECT 1
						FROM events e
						WHERE e.span_id = s.span_id
							AND e.name = 'x'
						GROUP BY e.name
					)`,
			at:   "GROUP BY",
			want: "GROUP BY is not supported yet",
		},
		{
			name: "exists over an unknown relation",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND EXISTS (
						SELECT 1
						FROM logs l
						WHERE l.span_id = s.span_id
					)`,
			at:   "logs",
			want: `unknown relation "logs"`,
		},
		{
			name: "correlation with an unknown inner column",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND EXISTS (
						SELECT 1
						FROM events e
						WHERE e.foo = s.span_id
					)`,
			at:   "e.foo",
			want: `events has no column "foo"`,
		},
		{
			name: "correlation with an unknown outer column",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND EXISTS (
						SELECT 1
						FROM links l
						WHERE l.span_id = s.foo
					)`,
			at:   "l.span_id",
			want: `"span_id" is a correlation column, not a predicate`,
		},
		{
			name: "links unknown column",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND EXISTS (
						SELECT 1
						FROM links l
						WHERE l.span_id = s.span_id
							AND l.foo = 1
					)`,
			at:   "l.foo",
			want: `links has no column "foo"`,
		},
		{
			name: "event column compared with the outer span",
			sql: `
				SELECT DISTINCT trace_id
				FROM spans s
				WHERE start_time >= now() - INTERVAL '1' HOUR
					AND start_time < now()
					AND EXISTS (
						SELECT 1
						FROM events e
						WHERE e.span_id = s.span_id
							AND e.name = s.name
					)`,
			at:   "s.name",
			want: "a predicate inside EXISTS reads the events columns only",
		},
		{
			name: "traces aggregate",
			sql: `
				SELECT count(*)
				FROM traces
				WHERE trace_id IN (
					SELECT trace_id
					FROM spans
					WHERE start_time >= now() - INTERVAL '1' HOUR
						AND start_time < now()
				)`,
			at:   "count",
			want: "count() is an aggregate",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Compile(tt.sql, now)
			require.Error(t, err)
			var refusal *Error
			require.ErrorAs(t, err, &refusal)
			assert.Contains(t, err.Error(), tt.want)
			offset := len(tt.sql)
			if tt.at != "" {
				offset = strings.Index(tt.sql, tt.at)
				require.NotEqual(t, -1, offset, "marker %q is not in the statement", tt.at)
			}
			line, column := lineColumn(tt.sql, offset)
			assert.Equal(t, line, refusal.Line, err.Error())
			assert.Equal(t, column, refusal.Column, err.Error())
		})
	}
}

// lineColumn converts a byte offset into the 1-based line and column the error reports.
func lineColumn(sql string, offset int) (line, column int) {
	before := sql[:offset]
	line = 1 + strings.Count(before, "\n")
	column = offset - strings.LastIndex(before, "\n")
	return line, column
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
