# Replacing `olivere/elastic` — Research & Migration Roadmap

**Related issue:** [#7612](https://github.com/jaegertracing/jaeger/issues/7612)  
**Author:** Amaan729  
**Status:** Draft — feedback welcome

---

## Background

Jaeger's Elasticsearch and OpenSearch storage backend currently depends on
[`github.com/olivere/elastic`](https://github.com/olivere/elastic), a
community-maintained Go client that is now **deprecated and unmaintained**.
The consequence is that bugs in this layer — including those caused by API
changes in upstream ES/OS — literally cannot be fixed without replacing the
dependency.

All callers go through the thin shim at
[`internal/storage/elasticsearch/client/interfaces.go`](../internal/storage/elasticsearch/client/interfaces.go),
which defines three interfaces:

```go
type IndexAPI interface {
    GetJaegerIndices(prefix string) ([]Index, error)
    IndexExists(index string) (bool, error)
    AliasExists(alias string) (bool, error)
    DeleteIndices(indices []Index) error
    CreateIndex(index string) error
    CreateAlias(aliases []Alias) error
    DeleteAlias(aliases []Alias) error
    CreateTemplate(template, name string) error
    Rollover(rolloverTarget string, conditions map[string]any) error
}

type ClusterAPI interface {
    Version() (uint, error)
}

type IndexManagementLifecycleAPI interface {
    Exists(name string) (bool, error)
}
```

Replacing `olivere/elastic` means providing new implementations of these three
interfaces backed by a modern client — everything else in the codebase stays
the same.

---

## Candidate Clients

### 1. `elastic/go-elasticsearch` (official Elastic client)

- **Repo:** https://github.com/elastic/go-elasticsearch
- **Status:** Actively maintained by Elastic. Supports ES 7.x and 8.x via
  separate major-version modules.
- **API style:** Functional-options pattern; returns `*esapi.Response` (HTTP
  response wrapper). Callers must decode JSON themselves.
- **Pros:** Official support, full ES API coverage, long-term commitment.
- **Cons:** Verbose — every call requires manual JSON marshalling/unmarshalling.
  API surface is very large; no higher-level query builder.

### 2. `opensearch-project/opensearch-go` (official OpenSearch client)

- **Repo:** https://github.com/opensearch-project/opensearch-go
- **Status:** Actively maintained by AWS/OpenSearch community.
- **API style:** Mirrors `go-elasticsearch` v7 closely (forked from it).
- **Pros:** Drop-in compatibility for our OS-specific e2e workflow. AWS SigV4
  signing built in.
- **Cons:** ES 8.x support is incomplete; not suitable as the sole client if we
  need to keep supporting upstream Elasticsearch.

---

## Interface Method Mapping

The table below maps each method in the shim to the equivalent call in each
candidate client.

| Shim method | `elastic/go-elasticsearch` | `opensearch-project/opensearch-go` |
|---|---|---|
| `GetJaegerIndices(prefix)` | `IndicesGet` with wildcard pattern | `IndicesGet` (identical API) |
| `IndexExists(index)` | `IndicesExists` | `IndicesExists` |
| `AliasExists(alias)` | `IndicesExistsAlias` | `IndicesExistsAlias` |
| `DeleteIndices(indices)` | `IndicesDelete` | `IndicesDelete` |
| `CreateIndex(index)` | `IndicesCreate` | `IndicesCreate` |
| `CreateAlias(aliases)` | `IndicesPutAlias` | `IndicesPutAlias` |
| `DeleteAlias(aliases)` | `IndicesDeleteAlias` | `IndicesDeleteAlias` |
| `CreateTemplate(tpl, name)` | `IndicesPutIndexTemplate` (v8) / `IndicesPutTemplate` (v7) | `IndicesPutTemplate` |
| `Rollover(target, conditions)` | `IndicesRollover` | `IndicesRollover` |
| `Version()` | `Info` — parse `version.number` | `Info` — identical |
| `ILM.Exists(name)` | `ILMGetLifecycle` | Not directly available; approximate with `ISM` |

**Key finding:** All `IndexAPI` and `ClusterAPI` methods have direct equivalents
in both clients. The only non-trivial gap is `IndexManagementLifecycleAPI`:
Elasticsearch uses ILM (Index Lifecycle Management) while OpenSearch uses ISM
(Index State Management) — these have different API shapes and will need
separate shim implementations.

---

## Recommended Path Forward

### Phase 1 — Dual-client shim (this PR establishes the plan)

Introduce a build-tag–selected implementation:

```
internal/storage/elasticsearch/client/
  interfaces.go            ← unchanged
  es/
    client.go              ← implements IndexAPI + ClusterAPI via go-elasticsearch v8
  os/
    client.go              ← implements IndexAPI + ClusterAPI via opensearch-go v2
```

Both implementations satisfy the same interfaces; the factory function selects
the right one based on the `backend` config field (`elasticsearch` vs
`opensearch`).

### Phase 2 — ILM / ISM split

`IndexManagementLifecycleAPI` gets two separate implementations:
`es/ilm.go` and `os/ism.go`. This is the most complex part and should be a
separate PR with dedicated integration tests.

### Phase 3 — Remove `olivere/elastic`

Once Phase 1 + 2 are merged and all e2e tests green, delete the old
implementation and drop the dependency from `go.mod`.

---

## Prototype: `ClusterAPI.Version()` via `go-elasticsearch`

Below is a minimal proof-of-concept showing how to implement the simplest shim
method with the new client. It is **not** wired into the build yet — it lives
here to validate the approach.

```go
// internal/storage/elasticsearch/client/es/client.go
package es

import (
    "encoding/json"
    "fmt"
    "strconv"
    "strings"

    es8 "github.com/elastic/go-elasticsearch/v8"
)

// Client wraps the official Elastic v8 client and satisfies client.IndexAPI
// and client.ClusterAPI.
type Client struct {
    es *es8.Client
}

// New returns a Client configured with the provided options.
func New(cfg es8.Config) (*Client, error) {
    c, err := es8.NewClient(cfg)
    if err != nil {
        return nil, err
    }
    return &Client{es: c}, nil
}

// Version implements client.ClusterAPI.
func (c *Client) Version() (uint, error) {
    res, err := c.es.Info()
    if err != nil {
        return 0, fmt.Errorf("fetching cluster info: %w", err)
    }
    defer res.Body.Close()

    if res.IsError() {
        return 0, fmt.Errorf("cluster info returned HTTP %s", res.Status())
    }

    var info struct {
        Version struct {
            Number string `json:"number"`
        } `json:"version"`
    }
    if err := json.NewDecoder(res.Body).Decode(&info); err != nil {
        return 0, fmt.Errorf("decoding cluster info: %w", err)
    }

    major := strings.SplitN(info.Version.Number, ".", 2)[0]
    v, err := strconv.ParseUint(major, 10, 32)
    if err != nil {
        return 0, fmt.Errorf("parsing major version %q: %w", major, err)
    }
    return uint(v), nil
}
```

---

## Open Questions

1. **ES 7 vs 8 support:** The `go-elasticsearch` v7 and v8 modules are separate
   Go modules with different import paths. Does Jaeger still need to support
   Elasticsearch 7? If yes, the factory needs to handle both.

2. **Authentication:** `olivere/elastic` has its own HTTP transport. The new
   clients support custom `http.RoundTripper`; existing auth/TLS config will
   need to be plumbed through.

3. **Bulk indexing:** Span writes use `olivere`'s `BulkService`. The new
   clients have a `BulkIndexer` (go-elasticsearch) and `BulkIndexer`
   (opensearch-go) that are functionally similar but differ in callback
   signatures. This is out of scope for this PR but must be tracked.

---

## Next Steps

- [ ] Get maintainer sign-off on the dual-client architecture.
- [ ] Raise separate issues / PRs for Phase 1, Phase 2, Phase 3.
- [ ] Add unit tests for the `Version()` prototype above.
- [ ] Investigate bulk-indexer API differences (tracked in a follow-up issue).
