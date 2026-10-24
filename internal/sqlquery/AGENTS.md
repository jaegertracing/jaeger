# Jaeger SQL

Search traces with one read-only `SELECT`. The statement is compiled to Jaeger's filter; nothing is executed as SQL.

## Three statement shapes

Trace IDs of the traces that have a matching span:

```sql
SELECT DISTINCT trace_id
FROM spans s
WHERE between(s.start_time, now() - '1h', now())   -- required: both bounds
  AND s.service = 'checkout'
  AND s.duration > '500ms'
  AND s.attributes['http.status_code'] >= 500
LIMIT 20
```

The matching spans themselves, in an order:

```sql
SELECT *
FROM spans s
WHERE between(s.start_time, now() - '1h', now())
  AND s.service = 'checkout'
ORDER BY s.duration DESC     -- start_time, duration, trace_id, span_id
LIMIT 100
```

Whole traces, selected by the spans they contain. The subquery is a spans query as above; the outer `WHERE` takes nothing else:

```sql
SELECT *
FROM traces
WHERE trace_id IN (
  SELECT trace_id
  FROM spans s
  WHERE between(s.start_time, now() - '1h', now())
    AND s.service = 'checkout'
)
LIMIT 20
```

## Columns of `spans`

`trace_id`, `span_id`, `parent_span_id`, `name`, `kind`, `start_time`, `end_time`, `duration`, `status`, `status_message`, `trace_state`, `service`, `scope_name`, `scope_version`, and the maps `attributes['key']`, `resource_attributes['key']`, `scope_attributes['key']`. `attr('key')` searches span and resource attributes together.

## Values

- Strings `'text'`. Numbers. `true`, `false`.
- Durations in Go syntax: `'2s'`, `'250ms'`, `'1h30m'`. A duration needs a unit.
- Instants in RFC 3339: `'2026-10-23T12:00:00Z'`, or `now() - '1h'`.
- `kind` is one of `server`, `client`, `producer`, `consumer`, `internal`, `unspecified`. `status` is `ok`, `error` or `unset`.

## Predicates

`=`, `<>`, `<`, `<=`, `>`, `>=`, `IN (...)`, `NOT IN`, `between(x, a, b)`, `AND`, `OR`, `NOT`, parentheses.

- Text search: `x LIKE 'GET %'` with `%` and `_` as in SQL (a backslash escapes the character after it), `x ILIKE 'get %'` to ignore case, or `x REGEXP 'pat'` / `match(x, 'pat')` in RE2 syntax. A regex matches anywhere in the value unless it starts with `^` or ends with `$` (anchors elsewhere are refused), and is case-sensitive unless it starts with `(?i)`.
- Presence: `attributes['key'] IS NOT NULL`. Built-in fields always exist.
- Events and links of the span: `EXISTS (SELECT 1 FROM events WHERE name = 'exception' AND attributes['exception.type'] = 'TimeoutError')`. Inside `EXISTS`, `events` and `links` are this span's own; no join condition is needed. Event columns: `name`, `time`, `time_since_start`, `attributes`. Link columns: `linked_trace_id`, `linked_span_id`, `trace_state`, `attributes`.

## Not supported

`JOIN`, `GROUP BY`, aggregates, `OFFSET`, arithmetic, `CAST`, other functions, and any relation other than `spans` and `traces` in `FROM`. An error names the construct, its line and column, and what to write instead.
