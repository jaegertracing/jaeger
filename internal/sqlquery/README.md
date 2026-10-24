# SQL query compiler

This package compiles a read-only SQL statement into the query that Jaeger's trace search RPCs take: the RFC 0005 filter, the start-time window, the RFC 0016 ordering and page size, and the RPC that the statement's shape selects. It is the implementation of [RFC 0019](../../docs/rfc/0019-sql-query-surface.md), and this page documents the dialect as the compiler accepts it. The RFC holds the reasoning.

The dialect is one `SELECT` over a virtual schema. Nothing is a physical table; every name compiles to a level, a field, or an RPC of the existing API, so a statement reaches every storage backend through the filter the backend already evaluates.

## Relations

| Relation | One row per | Columns |
|---|---|---|
| `spans` | span | `trace_id`, `span_id`, `parent_span_id`, `name`, `kind`, `start_time`, `end_time`, `duration`, `status`, `status_message`, `trace_state`, `service`, `scope_name`, `scope_version`, `attributes[...]`, `resource_attributes[...]`, `scope_attributes[...]` |
| `events` | span event | `name`, `time`, `time_since_start`, `attributes[...]` |
| `links` | span link | `linked_trace_id`, `linked_span_id`, `trace_state`, `attributes[...]` |
| `traces` | trace | `trace_id`, `root_service`, `root_name`, `start_time`, `duration`, `span_count`, `error_count` |

A statement reads `spans` or `traces` in `FROM`, optionally aliased. `events` and `links` appear only inside `EXISTS`, where they are the current span's own collections. The column names are the RFC 0005 field names in `snake_case`; the `camelCase` spelling of the IDL (`startTime`, `traceID`) is accepted and means the same column.

The three `*_attributes` columns are maps addressed by subscript, `attributes['http.status_code']`, and compile to an attribute reference at the span, resource or scope level. `attr('key')` names an attribute without a level, which a backend looks for at the span and resource levels; it is the filter counterpart of the legacy attributes map.

## Statement shape selects the RPC

| Statement | RPC | Fields filled |
|---|---|---|
| `SELECT DISTINCT trace_id FROM spans WHERE ...` | `FindTraceIDs` | filter, window, limit |
| `SELECT * FROM spans WHERE ...` | `FindSpans` | filter, window, order, limit |
| `SELECT span_id, duration, ... FROM spans WHERE ...` | `FindSpans` | the above, plus a projection of the listed columns |
| `SELECT * FROM traces WHERE trace_id IN (SELECT trace_id FROM spans WHERE ...)` | `FindTraces` | the subquery's filter and window; the outer `LIMIT` is the search depth |
| `SELECT trace_id, root_service, ... FROM traces WHERE trace_id IN (...)` | `FindTraceSummaries` | as above |

`ORDER BY` is accepted on a span search only, over `start_time`, `duration`, `trace_id` and `span_id`, each `ASC` or `DESC`. `LIMIT n` is the page size, or the search depth of a trace search; the server caps it. `OFFSET` is refused because the API paginates with the page token the response carries.

## The time window

Every search needs both bounds on `start_time`. The compiler takes them out of the top-level `AND` of `WHERE` and puts them in the request's window, so the filter never carries them:

```sql
WHERE between(start_time, now() - '1h', now())
```

or, with the bounds as two conjuncts, `start_time >= now() - '1h' AND start_time < now()`, which is the form to use when the end has to be exclusive.

A bound is an RFC 3339 instant, `'2026-10-23T12:00:00Z'`, or `now()` with a duration added or subtracted. The duration is written in Go syntax, `'1h'`, `'90m'`, `'2h30m'`, the same syntax every duration constant uses; the SQL-standard `INTERVAL '1' HOUR` is accepted as well. Both spellings of `BETWEEN` set both bounds at once. A `start_time` predicate that is not a bound, inside `OR` or compared with another column, stays in the filter.

## Predicates

| SQL | Compiles to | Notes |
|---|---|---|
| `AND`, `OR`, `NOT`, parentheses | `and`, `or`, `not` | flattened as the Go builder flattens |
| `=`, `==`, `<>`, `!=`, `<`, `<=`, `>`, `>=` | `eq`, `ne`, `lt`, `lte`, `gt`, `gte` | a constant on the left is mirrored to the right |
| `x IN (a, b)`, `x NOT IN (a, b)` | `in`, `not_in` | literal lists only |
| `x REGEXP 'pattern'`, `match(x, 'pattern')` | `regex` | RE2 syntax, matched anywhere in the value; anchors and case folding are refused by RFC 0005 |
| `x LIKE '%text%'` | `regex` | `%` becomes `.*` and `_` becomes `.` between the two `%` |
| `x LIKE 'text'` | `eq` | a pattern without a wildcard is an equality |
| `x IS NOT NULL`, `x IS NULL` | `exists`, `not(exists)` | on attributes only; a built-in field is always present |
| `between(x, a, b)`, `x BETWEEN a AND b` | `and(gte, lte)` | inclusive at both ends; the function form has no `AND` of its own, so it reads better beside other conjuncts. `NOT` negates either |
| `phrase(x, 'word word')`, `fulltext(x, 'word word')` | `phrase`, `fulltext` | on attributes; the words are split on whitespace |
| `EXISTS (SELECT 1 FROM events WHERE ...)` | `some` over events | the span's own events; see below |
| `EXISTS (SELECT 1 FROM links WHERE ...)` | `some` over links | the span's own links; see below |

A `LIKE` pattern anchored at one end, `LIKE 'GET%'`, is refused: the `regex` operator matches anywhere in the value and cannot express a prefix. `ILIKE` is refused because `regex` is case-sensitive. Both refusals say so.

Two columns may be compared, `start_time < end_time`, `attributes['after'] > attributes['before']`.

### Literals

Strings are single-quoted, with `''` or `\'` for a quote inside. Numbers are integers or decimals. Booleans are `true` and `false`. A constant carries no type; the field beside it types it, as RFC 0005 §5.4 specifies:

| Compared with | Accepted text |
|---|---|
| `start_time`, `end_time`, `e.time` | RFC 3339, `'2026-10-23T12:00:00Z'` |
| `duration`, `e.time_since_start` | Go duration, `'2s'`, `'250ms'`; `INTERVAL '250' MILLISECOND` is rewritten to it |
| `kind` | `'unspecified'`, `'internal'`, `'server'`, `'client'`, `'producer'`, `'consumer'` |
| `status` | `'unset'`, `'ok'`, `'error'` |
| other built-in fields | any text |
| an attribute | any text, matched at the type storage holds |

`duration > 2000` is refused because a duration needs a unit. `attributes['retries'] > 3` is passed through for storage to compare numerically.

### Events and links

A span's events and links are collections, and a conjunction of two event predicates has to bind to the same event. The dialect uses SQL's existential for that, and scopes it: **inside `EXISTS`, `events` and `links` are the current span's own collections**, not a relation of every event in storage. The subquery asks whether the span has an event that satisfies the predicate.

```sql
SELECT DISTINCT trace_id
FROM spans
WHERE between(start_time, now() - '1h', now())
  AND service = 'checkout'
  AND EXISTS (
    SELECT 1
    FROM events
    WHERE name = 'exception'
      AND attributes['exception.type'] = 'TimeoutError'
  )
```

That compiles to `some(events, and(...))`. The correlation a SQL reader expects, `e.span_id = s.span_id` and optionally `e.trace_id = s.trace_id`, is accepted and dropped, because the scoping already supplies it. Any other reference to the outer span inside the subquery is refused. The select list is `SELECT 1`; the parser does not accept `SELECT *` there. `NOT EXISTS` negates the quantifier.

Because the collection is the span's own, an `EXISTS` cannot ask about events of other spans in the trace. That is RFC 0005's structural tier, which no backend serves.

## What is refused

The compiler refuses, with a message that names the construct and the admitted alternative:

- `JOIN` of any kind, `WITH`, `UNION`, derived tables and table functions in `FROM`, `events` or `links` in the outer `FROM`. A statement reads one relation.
- `GROUP BY`, `HAVING` and aggregate functions, which are reserved for the row-returning RPC of RFC 0016 §5.
- `OFFSET`, `LIMIT BY`, `TOP`, `DISTINCT ON`, `WINDOW`, `PREWHERE`, `SETTINGS`, `FORMAT`.
- Arithmetic, `CAST`, string functions and any function other than `between`, `match`, `phrase`, `fulltext`, `attr` and `now`.
- A `traces` query whose `WHERE` is anything but `trace_id IN (SELECT trace_id FROM spans WHERE ...)`.
- A statement without both `start_time` bounds.

Three spellings are parse errors rather than refusals, because the parser library does not know them: PostgreSQL's `~` operator, the SQL-92 `TIMESTAMP '...'` literal, and `EXISTS (SELECT * ...)`.

## Errors

Every refusal is an `*Error` with the line and column of the construct, a message saying what is wrong, and a hint saying what the dialect admits instead:

```
line 4:7: the function lower() is not supported. The functions are between(column, low, high), match(column, 'regex'), phrase(column, 'words'), fulltext(column, 'words'), attr('key') and now()
```

The message is what an agent reads to repair its statement, so every message in the compiler is exercised by a statement in `compile_test.go` and asserted on its text and position.

## Usage

```go
query, err := sqlquery.Compile(sql, time.Now())
```

`now` is the instant `now()` evaluates to, fixed once per request so that the two bounds of a window agree. The result holds the RPC kind, the finalized filter, the window, the span order, the projection, the limit and the statement printed back in the parser's canonical spelling.
