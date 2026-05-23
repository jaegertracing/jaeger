# Jaeger API v3 HTTP query parameters

API v3 HTTP endpoints use **camelCase** query parameter names aligned with the [proto3 JSON mapping](https://protobuf.dev/programming-guides/proto3/#json) and the [OpenAPI specification](https://github.com/jaegertracing/jaeger-idl/blob/main/swagger/api_v3/query_service.openapi.yaml) (`naming=json`).

## Migration table

| Deprecated (snake_case) | Canonical (camelCase) | Endpoint(s) | Sunset version |
|---|---|---|---|
| `start_time` | `startTime` | `GET /api/v3/traces/{traceId}` | v2.20 |
| `end_time` | `endTime` | `GET /api/v3/traces/{traceId}` | v2.20 |
| `raw_traces` | `rawTraces` | `GET /api/v3/traces/{traceId}` | v2.20 |
| `query.service_name` | `query.serviceName` | `GET /api/v3/traces` | v2.20 |
| `query.operation_name` | `query.operationName` | `GET /api/v3/traces` | v2.20 |
| `query.start_time_min` | `query.startTimeMin` | `GET /api/v3/traces` | v2.20 |
| `query.start_time_max` | `query.startTimeMax` | `GET /api/v3/traces` | v2.20 |
| `query.duration_min` | `query.durationMin` | `GET /api/v3/traces` | v2.20 |
| `query.duration_max` | `query.durationMax` | `GET /api/v3/traces` | v2.20 |
| `query.raw_traces` | `query.rawTraces` | `GET /api/v3/traces` | v2.20 |
| `query.search_depth` | `query.searchDepth` | `GET /api/v3/traces` | v2.20 |
| `query.num_traces` | `query.searchDepth` | `GET /api/v3/traces` | v2.20 |
| `span_kind` | `spanKind` | `GET /api/v3/operations` | v2.20 |

`query.num_traces` is a **semantic rename** from Jaeger API v2 (`num_traces`), not merely a snake_case variant. Both `query.num_traces` and `query.search_depth` are deprecated in favor of `query.searchDepth`, which matches the proto field `search_depth`.

## Deprecation signals

When deprecated parameters are used, responses include:

- `Deprecation: true`
- `Sunset: Mon, 01 Nov 2026 00:00:00 GMT` (target removal in Jaeger v2.20.0)
- `Link: <.../docs/apis/api_v3_http.md>; rel="deprecation"`
- `Deprecated-Params: <comma-separated names>`

The query service logs a single WARN line per request (no parameter values are logged).

## Precedence rules

1. **Canonical wins** — if both `startTime` and `start_time` are sent, `startTime` is used and no deprecation headers are emitted.
2. **Empty values are absent** — `?start_time=` is ignored.
3. **First value wins** — `?startTime=a&startTime=b` uses `a`.

## Related

- [Issue #8619](https://github.com/jaegertracing/jaeger/issues/8619)
- Companion OpenAPI fix: [jaeger-idl#202](https://github.com/jaegertracing/jaeger-idl/pull/202)
- Jaeger UI client migration: [jaeger-ui#3947](https://github.com/jaegertracing/jaeger-ui/pull/3947)
