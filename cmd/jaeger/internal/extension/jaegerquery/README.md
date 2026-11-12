# jaeger_query

This extension implements traditional `jaeger-query` service (including Jaeger UI). Because it is not a part of the collector pipeline, it needs access to a storage backend, which can be potentially shared with an exporter (e.g. in order to implement the `all-in-one` functionality). For this reason it depends on the [jaeger_storage](../jaegerstorage/) extension.

## Configuration

This is work in progress, most of the usual settings of `jaeger-query` are not supported yet.

```yaml
jaeger_query:
    trace_storage: cassandra_primary
    trace_archive: cassandra_archive
    dependencies: memstore
    metrics_store: prometheus_store
```

## Authentication and data policy

`jaeger-query` has no login of its own. The deployment pattern that gives it per-user
behavior, an identity-aware proxy in front of the query port and an authenticator extension
that turns its headers into a caller, is described on the website's
[security page](https://www.jaegertracing.io/docs/latest/deployment/security/). This section
records what the code guarantees to such an extension.

An authenticator extension implements the collector's `extensionauth.Server` interface. The
extension in `jaeger_query` that resolves `query_interceptors` from the collector host also
resolves the authenticator, so an installation builds both into its own Jaeger binary.
`http.auth` applies the authenticator to every route on the port; `ai.auth` applies it to the
AI surfaces alone. The authenticator sees headers and the query parameters listed in
`request_params`, never the URL path.

A `query_interceptors` extension runs around every trace query the query service executes,
whether the request came from the UI, the REST API or an MCP tool, and reads the caller from
the request context. The tool calls that a chat turn makes are the exception: the agent dials
the turn-scoped MCP endpoint with no credentials, and the caller admitted on the chat request
is not carried onto those calls, so an interceptor sees an anonymous caller there.

The `bearer_token_propagation` and `header_forwarding` settings are a different mechanism:
they pass selected headers through to the storage backend, for a backend that does its own
per-user authorization, and do not authenticate anything in Jaeger.
