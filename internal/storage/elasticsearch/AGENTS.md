# AGENTS.md — Elasticsearch/OpenSearch storage

Scoped guidance for the ES/OS storage packages. The root [AGENTS.md](../../../AGENTS.md) still applies; this file adds conventions specific to this subtree.

## Testing request wire formats

When a test needs to assert what an operation sends on the wire (HTTP method, path, query, and the JSON or `_bulk`/`_msearch` NDJSON body), use the [`snapshottest`](./snapshottest) harness and commit a golden fixture under the package's `testdata/` — do **not** hand-write `assert.Contains(body, ...)` / `assert.Equal` checks against the request body. Golden fixtures make the full request visible and diffable, canonicalize key order so assertions don't depend on map iteration, and cover multi-request cases (e.g. how a batch splits into chunks) that ad-hoc substring checks obscure.

Pattern: drive the operation against a `snapshottest.NewRecorder` server, then

```go
rec.Assert(t, "testdata/<subject>")                       // one wire format for all backends/versions
snapshottest.AssertByVersion(t, "testdata/<subject>", m)  // when ES/OS versions emit different requests
```

Regenerate (and range-collapse) fixtures after an intentional change:

```bash
REGENERATE_SNAPSHOTS=true go test ./internal/storage/elasticsearch/...
```

Review the regenerated `testdata/` diff before committing — an unexpected change there is a wire-format regression, which is the whole point of the snapshot. Asserting on returned errors, metrics, or parsed results (not the request body) stays ordinary `assert`/`require`.
