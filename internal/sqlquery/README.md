# SQL query compiler

This package compiles a read-only SQL statement into the parameters that the query service's search methods take: the RFC 0005 filter, the start-time window, the RFC 0016 ordering and page size, and the method that the statement's shape selects. The compiler runs in process; no remote call is involved. It is the implementation of [RFC 0019](../../docs/rfc/0019-sql-query-surface.md), and this page documents the dialect as the compiler accepts it. The RFC holds the reasoning.

The dialect is one `SELECT` over a virtual schema. Nothing is a physical table; every name compiles to a level, a field, or a search method of the query service, so a statement reaches every storage backend through the filter the backend already evaluates.

## Relations

A statement reads `spans` or `traces` in `FROM`, optionally aliased. `events` and `links` appear only inside `EXISTS`, where they are the current span's own collections. A column name is the RFC 0005 field name in `snake_case`, matched exactly; `startTime` and `TRACE_ID` are not columns.

### `spans`, one row per span

| Column | Type | Values |
|---|---|---|
| `trace_id`, `span_id`, `parent_span_id` | text | lower-case hex; a root span has no `parent_span_id`, and no comparison on it matches a root span |
| `name`, `status_message` | text | any |
| `trace_state` | text | any; a span without a trace state has no value, so no comparison on it matches that span |
| `kind` | word | `'server'`, `'client'`, `'producer'`, `'consumer'`, `'internal'`, `'unspecified'` |
| `status` | word | `'ok'`, `'error'`, `'unset'` |
| `start_time`, `end_time` | instant | RFC 3339, `'2026-10-23T12:00:00Z'`, or `now() - '1h'` |
| `duration` | duration | Go syntax with a unit, `'2s'`, `'250ms'`; `INTERVAL '250' MILLISECOND` is rewritten to it |
| `service` | text | the resource's `service.name` |
| `scope_name`, `scope_version` | text | the instrumentation scope |
| `attributes['key']` | map | span attributes; a value is compared at the type storage holds |
| `resource_attributes['key']` | map | resource attributes |
| `scope_attributes['key']` | map | instrumentation scope attributes |

The three map columns compile to an attribute reference at the span, resource or scope level. The dialect has no level-less attribute reference: the span-or-resource search of RFC 0005 §5.1 is not exposed, so that every attribute predicate says which level it reads.

### `events` and `links`, inside `EXISTS` only

| Relation | Column | Type | Values |
|---|---|---|---|
| `events` | `name` | text | any |
| `events` | `time` | instant | RFC 3339 |
| `events` | `time_since_start` | duration | Go syntax with a unit |
| `events` | `attributes['key']` | map | event attributes |
| `links` | `linked_trace_id`, `linked_span_id` | text | lower-case hex |
| `links` | `trace_state` | text | any |
| `links` | `attributes['key']` | map | link attributes |

The `span_id` and `trace_id` of an event or link appear only in the optional correlation with the outer span, `e.span_id = s.span_id`; a predicate on them is refused, because inside `EXISTS` the collection is already the span's own.

### `traces`, one row per trace

| Column | Type |
|---|---|
| `trace_id` | text |
| `root_service`, `root_name` | text |
| `start_time` | instant |
| `duration` | duration |
| `span_count`, `error_count` | number |

The columns of `traces` are the RFC 0011 summary fields. They may be selected but not filtered on, because the expression tree has no trace-level fields; the only `WHERE` on `traces` is `trace_id IN (SELECT trace_id FROM spans WHERE ...)`.

## Statement shape selects the method

| Statement | Method | Fields filled |
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

or, with the bounds as two conjuncts, `start_time >= now() - '1h' AND start_time < now()`. The window the request carries is inclusive at both ends, so a strict bound moves its instant by one nanosecond, the resolution of a span's start time; `<` therefore excludes the instant it names.

A bound is an RFC 3339 instant, `'2026-10-23T12:00:00Z'`, or `now()` with a duration added or subtracted. The duration is written in Go syntax, `'1h'`, `'90m'`, `'2h30m'`, the same syntax every duration constant uses; the SQL-standard `INTERVAL '1' HOUR` is accepted as well. Both spellings of `BETWEEN` set both bounds at once. A `start_time` predicate that is not a bound, inside `OR` or compared with another column, stays in the filter.

## Predicates

| SQL | Compiles to | Notes |
|---|---|---|
| `AND`, `OR`, `NOT`, parentheses | `and`, `or`, `not` | flattened as the Go builder flattens |
| `=`, `==`, `<>`, `!=`, `<`, `<=`, `>`, `>=` | `eq`, `ne`, `lt`, `lte`, `gt`, `gte` | a constant on the left is mirrored to the right |
| `x IN (a, b)`, `x NOT IN (a, b)` | `in`, `not_in` | literal lists only |
| `x REGEXP 'pattern'`, `match(x, 'pattern')` | `regex` | RE2 syntax, matched anywhere in the value unless the pattern starts with `^` or ends with `$` (an anchor anywhere else is refused); `(?i)` folds case |
| `x LIKE 'GET %'`, `x LIKE '%.json'` | `regex` | `%` and `_` match any characters, a newline included; a backslash escapes the character after it, so `\%` is the percent sign; the pattern is anchored at both ends |
| `x ILIKE 'get %'` | `regex` | as `LIKE`, with `(?i)` |
| `x LIKE 'text'` | `eq` | a pattern without a wildcard is an equality |
| `x IS NOT NULL`, `x IS NULL` | `exists`, `not(exists)` | on attributes only; a built-in field is always present |
| `between(x, a, b)`, `x BETWEEN a AND b` | `and(gte, lte)` | inclusive at both ends; the function form has no `AND` of its own, so it reads better beside other conjuncts. `NOT` negates either |
| `phrase(x, 'word word')`, `fulltext(x, 'word word')` | `phrase`, `fulltext` | on attributes; the words of every quoted argument are split on whitespace |
| `EXISTS (SELECT 1 FROM events WHERE ...)` | `some` over events | the span's own events; see below |
| `EXISTS (SELECT 1 FROM links WHERE ...)` | `some` over links | the span's own links; see below |

Two columns may be compared, `start_time < end_time`, `attributes['after'] > attributes['before']`.

### Literals

Strings are single-quoted, with `''` or `\'` for a quote inside and `\\` for a backslash. Any other backslash is a character of the text, so a regex is written as it is read, `'\d+'`. Numbers are integers or decimals. Booleans are `true` and `false`. A constant carries no type; the field beside it types it, as RFC 0005 §5.4 specifies:

| Compared with | Accepted text |
|---|---|
| `start_time`, `end_time`, `e.time` | RFC 3339, `'2026-10-23T12:00:00Z'`, or `now()` with a duration subtracted, `now() - '1h'` |
| `duration`, `e.time_since_start` | Go duration, `'2s'`, `'250ms'`; `INTERVAL '250' MILLISECOND` is rewritten to it, with `MILLISECOND`, `SECOND`, `MINUTE`, `HOUR`, `DAY` and `WEEK` as the units |
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
- `GROUP BY`, `HAVING` and aggregate functions, which are reserved for the row-returning query service method of RFC 0016 §5.
- `OFFSET`, `LIMIT BY`, `TOP`, `DISTINCT ON`, `WINDOW`, `PREWHERE`, `SETTINGS`, `FORMAT`, `WITH TOTALS`, `SAMPLE`, `FINAL`, `GLOBAL IN` and the `SELECT` modifiers `EXCEPT`, `REPLACE` and `APPLY`.
- Arithmetic, `CAST`, string functions and any function other than `between`, `match`, `phrase`, `fulltext` and `now`; `attr('key')` is refused with a hint naming the three map columns.
- A `traces` query whose `WHERE` is anything but `trace_id IN (SELECT trace_id FROM spans WHERE ...)`.
- A statement without both `start_time` bounds.

Four spellings are parse errors rather than refusals, because the parser library does not know them: PostgreSQL's `~` operator, the SQL-92 `TIMESTAMP '...'` literal, `EXISTS (SELECT * ...)`, and `x NOT REGEXP 'p'`, which is written `NOT x REGEXP 'p'`.

## Errors

Every refusal is an `*Error` with the line and column of the construct, a message saying what is wrong, and a hint saying what the dialect admits instead:

```
line 4:7: the function lower() is not supported. The functions are between(column, low, high), match(column, 'regex'), phrase(column, 'words'), fulltext(column, 'words') and now()
```

The message is what an agent reads to repair its statement, so the refusals in `compile_test.go` are asserted on their text and position. A filter the finalizer of RFC 0005 refuses, such as a regex with an anchor inside the pattern, is reported at the `WHERE` keyword, because the finalizer sees the tree and not the text.

## Usage

```go
query, err := sqlquery.Compile(sql, time.Now())
```

`now` is the instant `now()` evaluates to, fixed once per request so that the two bounds of a window agree. The result holds the method, the finalized filter, the window, the span order, the projection, the limit and the statement printed back in the parser's canonical spelling.
