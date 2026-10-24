---
name: jaeger-sql
description: >-
  Write the sql input of search_traces. Use when the user writes the search as
  SQL, or when the structured inputs cannot say it: OR, NOT, a comparison on an
  attribute, a text pattern, or a condition on a span's events or links.
allowed-tools: search_traces
---

# Jaeger SQL

Search traces with one read-only `SELECT` in the `sql` input of `search_traces`. The statement is compiled to Jaeger's filter; nothing is executed as SQL. `search_traces` returns traces, so it takes the first and third shapes below; it refuses the second, which is reserved for a span search that no tool serves yet.

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

## Columns

Column names are `snake_case` and match exactly.

### `spans`

| Column | Type | Values |
|---|---|---|
| `trace_id`, `span_id`, `parent_span_id` | text | lower-case hex; a root span has no `parent_span_id`, and no comparison on it matches a root span |
| `name`, `status_message`, `trace_state` | text | any |
| `kind` | word | `'server'`, `'client'`, `'producer'`, `'consumer'`, `'internal'`, `'unspecified'` |
| `status` | word | `'ok'`, `'error'`, `'unset'` |
| `start_time`, `end_time` | instant | RFC 3339 `'2026-10-23T12:00:00Z'`, or `now() - '1h'` |
| `duration` | duration | Go syntax with a unit: `'2s'`, `'250ms'`, `'1h30m'` |
| `service` | text | the service name |
| `scope_name`, `scope_version` | text | the instrumentation scope |
| `attributes['key']` | map | span attributes; the value is compared at the type storage holds |
| `resource_attributes['key']` | map | resource attributes |
| `scope_attributes['key']` | map | scope attributes |

Every attribute predicate names its level. When the level is unknown, `attributes['key']` is the first choice.

### `events` and `links`, inside `EXISTS` only

| Relation | Columns |
|---|---|
| `events` | `name` text, `time` instant, `time_since_start` duration, `attributes['key']` map |
| `links` | `linked_trace_id` and `linked_span_id` hex text, `trace_state` text, `attributes['key']` map |

### `traces`

`trace_id`, `root_service`, `root_name`, `start_time`, `duration`, `span_count`, `error_count`. These columns may be selected but not filtered on; the only `WHERE` on `traces` is `trace_id IN (SELECT trace_id FROM spans WHERE ...)`.

## Values

- Strings `'text'`. Numbers. `true`, `false`.
- A duration needs a unit. An instant is RFC 3339 or `now() - '1h'`.

## Predicates

`=`, `<>`, `<`, `<=`, `>`, `>=`, `IN (...)`, `NOT IN`, `between(x, a, b)`, `AND`, `OR`, `NOT`, parentheses.

- Text search: `x LIKE 'GET %'` with `%` and `_` as in SQL, `x ILIKE 'get %'` to ignore case, or `x REGEXP 'pat'` / `match(x, 'pat')` in RE2 syntax. A regex matches anywhere in the value unless it starts with `^` or ends with `$` (anchors elsewhere are refused), and is case-sensitive unless it starts with `(?i)`.
- Presence: `attributes['key'] IS NOT NULL`. Built-in fields always exist.
- Events and links of the span: `EXISTS (SELECT 1 FROM events WHERE name = 'exception' AND attributes['exception.type'] = 'TimeoutError')`. Inside `EXISTS`, `events` and `links` are this span's own; no join condition is needed.

## Not supported

`JOIN`, `GROUP BY`, aggregates, `OFFSET`, arithmetic, `CAST`, other functions, and any relation other than `spans` and `traces` in `FROM`. An error names the construct, its line and column, and what to write instead.
