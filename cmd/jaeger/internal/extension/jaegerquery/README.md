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

## Authenticating callers behind an identity-aware proxy

`jaeger-query` has no login of its own. The deployment pattern that gives it per-user
behavior puts an identity-aware proxy in front of the query port and lets the collector's
authenticator extensions and the `query_interceptors` do the rest:

1. **The proxy authenticates the person.** A single-sign-on proxy, an ingress with OIDC, or a
   service mesh terminates the user's session for every request to the UI, the REST API, the
   chat endpoint and the MCP endpoint alike, and injects the result as request headers: a
   signed identity token, or a plain username header. The browser never sees a challenge from
   Jaeger itself. A tool such as an MCP client or `curl` that bypasses the browser obtains the
   same token and sends it in a header up front.
2. **Jaeger trusts those headers only from the proxy.** The query port is reachable only
   through the proxy, by network policy or by mutual TLS, so a header that names a user is
   trustworthy because of where the request came from. Any deployment that exposes the port
   directly must not trust a username header at all, and should verify a signed token instead.
3. **An authenticator extension turns the headers into a caller.** The collector's
   `extensionauth.Server` interface receives the request headers, verifies the token or reads
   the trusted header, and returns a context that records who is calling, or an error that
   Jaeger answers with `401`. An installation builds this extension into its own Jaeger binary;
   the one in `jaeger_query` that resolves `query_interceptors` from the collector host also
   resolves the authenticator. The same extension can consult a group directory and reject a
   caller who is outside a rollout group, which is how a feature is opened to a few people
   first. `http.auth` applies the extension to every route on the port; `ai.auth` applies it
   to the AI surfaces alone, which is the right grain when the UI stays open to everyone.
4. **A query interceptor enforces data policy.** Who may see which spans is a question about
   the data rather than the route, so it belongs in a `query_interceptors` extension, which runs
   around every trace query the query service executes, whether the request came from the UI,
   the REST API or an MCP tool. Such an interceptor reads the caller from the request context
   and narrows the query or redacts the result per span. The tool calls that a chat turn makes
   are the exception today: the chat sidecar dials the turn-scoped MCP endpoint with no
   credentials, and the caller admitted on the chat request is not yet carried onto those
   calls, so an interceptor sees an anonymous caller there.

The authenticator sees headers and the query parameters listed in `request_params`, never the
URL path, which is why the per-route grain is a configuration setting rather than something the
extension decides. The `bearer_token_propagation` and `header_forwarding` settings are a
different mechanism: they pass selected headers through to the storage backend, for a backend
that does its own per-user authorization, and do not authenticate anything in Jaeger.
