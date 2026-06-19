# RFC 0004: Elasticsearch/OpenSearch Data Streams for Span Storage

- **Status:** Draft
- **Author:** Yuri Shkuro
- **Created:** 2026-06-18
- **Last Updated:** 2026-06-18
- **Issue:** [#4708](https://github.com/jaegertracing/jaeger/issues/4708)
- **Supersedes:** [Google Doc proposal](https://docs.google.com/document/d/1WDQJmHGjnyck5h1DDvZf4ZJnTaXEF-wUvR6k_lipWJ4), [Draft ADR PR #7974](https://github.com/jaegertracing/jaeger/pull/7974)

---

## Abstract

This RFC proposes adding **Data Streams** as a new (initially experimental) index management strategy for Jaeger's Elasticsearch/OpenSearch span storage. Data Streams are the native abstraction for append-only time-series data and eliminate the need for the `jaeger-es-rollover` initialization tool, manual alias management, and external cron jobs, while providing built-in lifecycle management.

The existing three strategies (time-based indices, manual rollover, rollover with ILM) remain fully supported. Data streams are opt-in via configuration and may become the recommended default in a future release once the community has gained operational experience.

The scope is intentionally limited: use existing Jaeger span schema (not native OTLP), use existing ES client library, target OpenSearch as primary backend.

---

## 1. Motivation

### Current Pain Points

Jaeger's ES backend currently offers three index management strategies, each with operational burden:

| Strategy | Operational Overhead |
|----------|---------------------|
| Time-based indices (default) | `jaeger-es-index-cleaner` cron job |
| Manual rollover | `jaeger-es-rollover init` + `rollover` cron + `lookback` cron + `index-cleaner` cron |
| Rollover with ILM | `jaeger-es-rollover init` + ILM policy creation |

Even the "simplest" ILM path requires a one-time initialization step that creates index templates, a seed index, and read/write aliases — a common source of misconfiguration.

### Why Data Streams

[Data Streams](https://www.elastic.co/guide/en/elasticsearch/reference/current/data-streams.html) (ES 7.9+, OpenSearch 2.0+) are purpose-built for append-only time-series data:

- **No initialization step**: Creating a composable index template with `data_stream: {}` is sufficient. The first write auto-creates the data stream and its first backing index.
- **No alias management**: The data stream name itself acts as both the read and write target.
- **Built-in rollover**: ILM/ISM policies referenced in the template automatically roll over backing indices.
- **Simplified writes**: All documents go to a single endpoint (`POST /<data-stream>/_doc`).
- **Transparent reads**: Queries against the data stream name automatically span all backing indices.

Spans are ideal candidates: they are immutable, append-only, and time-stamped.

---

## 2. Scope

### In Scope

- Data stream support for **spans** only
- OpenSearch as primary target (with Elasticsearch also supported)
- ISM policy management (create-on-startup if missing)
- Backward-compatible migration path from all existing modes
- Deprecation plan for `jaeger-es-rollover` tool

### Out of Scope (explicit non-goals)

- **Native OTLP schema**: The ES storage recently completed migration to v2 Storage API ([#6458](https://github.com/jaegertracing/jaeger/issues/6458)), but still uses the Jaeger `dbmodel` internally. Changing the on-disk schema to native OTLP (tracked by [#8078](https://github.com/jaegertracing/jaeger/issues/8078) for missing scope/link attributes) is orthogonal and should not block data stream adoption. Data streams work with any document schema.
- **olivere client replacement**: Replacing the olivere/elastic client ([#7612](https://github.com/jaegertracing/jaeger/issues/7612)) is independent. Data stream writes use the same bulk API — only the op_type changes from `index` to `create`. This is a one-line change regardless of which client library is used.
- **Services/Dependencies indices**: These require document updates (deduplication), which data streams do not support. They remain as standard indices. See §2.1 below for discussion of the operational implications.
- **LogsDB mode, searchable snapshots, advanced ILM phases**: These are ES-specific optimizations users can layer on via `@custom` component templates. Jaeger should not embed these in its configuration.

### Operational Implications for Services and Dependencies Indices

A valid concern is that moving spans to data streams while leaving services and dependencies on legacy indices creates a "dual-management" split. In practice, the burden is minimal:

**Dependencies index**: The dependencies index stores pre-aggregated service dependency graphs (computed by Spark/Flink jobs or the internal dependency calculator). Few deployments use it, and data volume is negligible — a handful of documents per computation interval. The default daily index strategy works without any cron jobs or lifecycle management. No action needed.

**Services index**: This is a deduplication cache of `{serviceName, operationName}` pairs (document ID = FNV hash, so writes are idempotent upserts). Data volume is bounded by `#services × #operations` — typically thousands of documents, not millions. The current daily index rotation provides implicit staleness cleanup: decommissioned services naturally disappear when their daily index ages out.

For data stream users, the services index can remain on the default `time_based` strategy with no additional tooling:
- Jaeger's built-in daily index creation handles rotation.
- The `jaeger-es-index-cleaner` (already needed during the span migration period for legacy span indices) handles cleanup.
- Once legacy span indices have aged out and `jaeger-es-index-cleaner` is no longer needed for spans, it could be retained solely for the services index — but given the trivial data volume, even leaving services indices indefinitely is harmless.

**Future improvement** (not in this RFC's scope): Keep daily rotation for the services index (it provides natural protection against high-cardinality poisoning from misbehaving instrumentation), but move the cleanup logic in-process — i.e., run the equivalent of `jaeger-es-index-cleaner` as a background goroutine inside Jaeger itself. This eliminates the last external cron job, completing the "zero-external-tooling" promise of data streams, without requiring a schema change or new cardinality safeguards. This is a straightforward follow-up that does not block the current work.

---

## 3. Design

### 3.1 Data Stream Naming

Use dot-notation names.

| Data Stream | Purpose |
|-------------|---------|
| `jaeger.spans` | Span documents |

Dot-notation (vs. `jaeger-ds-span`) aligns with ES/OpenSearch conventions and enables the `@custom` component template pattern: users can create `jaeger@custom` to override settings across all Jaeger data streams, or `jaeger.spans@custom` for span-specific overrides.

When `index_prefix` is configured, it becomes the top-level namespace: `<prefix>.jaeger.spans`. This means the prefix participates in the dot-notation hierarchy for `@custom` templates:

| `index_prefix` | Data stream | `@custom` overrides |
|----------------|-------------|---------------------|
| (none) | `jaeger.spans` | `jaeger.spans@custom`, `jaeger@custom` |
| `prod` | `prod.jaeger.spans` | `prod.jaeger.spans@custom`, `prod.jaeger@custom`, `prod@custom` |

This is desirable for multi-tenant clusters — e.g., a `prod@custom` template can set a policy across all `prod.*` data streams. Note that prefixes containing dashes (e.g., `my-team.jaeger.spans`) still work with the `@custom` pattern but lose hierarchical wildcard benefits at the prefix level.

Alternative considered: `jaeger-ds-span` (proposed in Google Doc). Rejected because the `-ds-` infix is non-standard and doesn't integrate with the `@custom` convention.

### 3.2 Index Template

#### Background: Composable Index Templates

ES/OpenSearch has two generations of index templates:
- **Legacy templates** (`_template` API): monolithic, cannot be composed. This is what Jaeger uses today.
- **Composable templates** (`_index_template` API, ES 7.8+/OS 2.0+): built by combining reusable **component templates**. The final template applied to an index is the merge of all referenced component templates plus inline settings. Data streams *require* composable templates (the legacy API does not support the `data_stream: {}` directive).

Component templates are reusable building blocks. By splitting mappings and settings into separate components, Jaeger enables users to override individual aspects without replacing the entire template (via the `@custom` naming convention — see below).

#### What Jaeger Creates on Startup

Jaeger creates three objects on startup (all PUT operations are idempotent — creating a template that already exists with the same content is a no-op, and creating one with updated content overwrites it).

In the examples below, `<prefix>` denotes the fully-resolved prefix **including the trailing dot** when `index_prefix` is set (e.g., `prod.`), or the empty string when no prefix is configured. See the "Index Prefix" table below for concrete examples.

**1. Component template: `<prefix>jaeger.spans@mappings`**

Contains field mappings (equivalent to current `jaeger-span-8.json` content plus `@timestamp`). Separated so that mapping changes across Jaeger versions are applied cleanly without touching user settings.

**2. Component template: `<prefix>jaeger.spans@settings`**

Contains the lifecycle policy reference and default shard/replica counts. Jaeger auto-detects the backend and emits the correct JSON setting internally (`index.lifecycle.name` for ES, or the ISM policy attachment for OpenSearch). This is transparent to the user — Jaeger configuration is identical regardless of backend.

**3. Composable index template: `<prefix>jaeger.spans`**

Ties everything together:

```json
{
  "index_patterns": ["<prefix>jaeger.spans"],
  "data_stream": {},
  "composed_of": [
    "<prefix>jaeger.spans@mappings",
    "<prefix>jaeger.spans@settings",
    "<prefix>jaeger.spans@custom"
  ],
  "priority": 500,
  "ignore_missing_component_templates": ["<prefix>jaeger.spans@custom"]
}
```

The `data_stream: {}` directive tells ES/OpenSearch that any write to a matching index pattern should be handled as a data stream (auto-creating backing indices, enforcing append-only semantics, etc.).

The `@custom` component template is explicitly listed in `composed_of` (last position = highest priority) but marked in `ignore_missing_component_templates` so that the index template is valid even when the user has not created it. This is required because OpenSearch does not auto-merge `@custom` templates — they must be explicitly referenced.

#### Idempotency and Conflict Handling

- Template creation is idempotent: PUT with the same name overwrites the previous version. This is safe because Jaeger controls these templates.
- **User customizations are never overwritten** because they live in a separate `<prefix>jaeger.spans@custom` component template which Jaeger does not touch. Since `@custom` is listed last in `composed_of`, its settings take highest priority when it exists.
- On startup, Jaeger always writes its templates (ensuring mappings stay current with the Jaeger version). This is the same behavior as the current `create_mappings: true` mode, applied to the new composable template format.

#### Index Prefix / Custom Names

Jaeger's existing `index_prefix` config (e.g., `index_prefix: "prod"`) applies to all template and data stream names:

| Config | Data stream name | Templates |
|--------|-----------------|-----------|
| (default) | `jaeger.spans` | `jaeger.spans@mappings`, `jaeger.spans@settings` |
| `index_prefix: "prod"` | `prod.jaeger.spans` | `prod.jaeger.spans@mappings`, `prod.jaeger.spans@settings` |

This preserves multi-tenancy support for shared ES clusters.

#### User Customization via `@custom` Pattern

Jaeger's composable index template includes a `<prefix>jaeger.spans@custom` component template reference (with `ignore_missing_component_templates` so it need not exist). When a user creates this component template, its settings are merged with highest priority (last in `composed_of` wins). Jaeger never creates or modifies this template — it is entirely user-controlled.

Example: a user wanting a different ILM policy creates:
```json
PUT _component_template/jaeger.spans@custom
{
  "template": {
    "settings": {
      "index.lifecycle.name": "my-custom-policy"
    }
  }
}
```

This overrides Jaeger's default policy without touching any Jaeger config or risking overwrites on upgrade. No Jaeger restart is required — the override takes effect on the next index created (i.e., the next rollover).

### 3.3 The `@timestamp` Field

Data streams require a `@timestamp` field mapped as `date` or `date_nanos`.

Recommendation: Add `@timestamp` as a field in the document at write time (in Go code), derived from the OTLP `StartTimestamp` at nanosecond precision. No ingest pipeline needed.

Rationale:
- OTLP defines timestamps in nanoseconds; truncating to milliseconds loses precision unnecessarily.
- ES/OpenSearch `date_nanos` type supports epoch nanoseconds natively.
- Ingest pipelines add operational complexity and a failure point.
- Avoids dependency on ES ingest nodes (relevant for cost in licensed ES/ECK deployments).

```go
// In the span-to-dbmodel conversion
doc["@timestamp"] = span.StartTimestamp().AsTime().UnixNano()
```

The field is added to the mapping component template:
```json
{
  "@timestamp": { "type": "date_nanos", "format": "epoch_nanos" }
}
```

Note: The existing `startTime` (microseconds) and `startTimeMillis` fields remain for backward compatibility with queries. `@timestamp` is used exclusively by the data stream machinery for rollover and time-based partitioning.

### 3.4 Write Path Changes

Current write path uses `BulkIndexRequest` (op_type=`index`) via the olivere client. The olivere `BulkIndexRequest` supports `.OpType("create")` — so switching to create semantics does not require a different request type or client library change, just adding `.OpType("create")` to the existing call chain.

Data streams require op_type=`create` (append-only, no upserts). The target index name becomes the data stream name (e.g., `jaeger.spans`) with no date suffix.

#### Duplicate Span Handling

Today, Jaeger does **not** set a document `_id` when writing spans to ES (see `writeSpanToIndex` in `writer.go`). ES auto-generates a random `_id` for each write. This means:

- **Current behavior**: Duplicate writes (e.g., from Kafka consumer retries) produce duplicate documents in ES. This is the existing behavior and is tolerated — queries may return duplicate spans but it does not cause data corruption.
- **With data streams**: The same behavior continues. Since no explicit `_id` is set, each write (including retries) creates a new document. op_type=`create` with an auto-generated ID always succeeds — it only rejects writes when an *explicit* ID already exists.

If deduplication is desired in the future, Jaeger could set `_id` to a deterministic value (e.g., `traceID + spanID + startTime`). With op_type=`create`, a retry with the same `_id` would be rejected by ES (409 Conflict) rather than creating a duplicate. However, this is an independent improvement and not required for data stream support.

**[FOR REVIEW]** Open question: Should we take this opportunity to set a deterministic `_id` and get at-least-once deduplication for free? Trade-off: setting `_id` disables ES auto-ID optimization (slightly slower bulk indexing) but eliminates duplicate spans from Kafka retries.

Recommendation: Do not change `_id` behavior in this RFC. Keep it as a follow-up improvement to avoid scope creep.

### 3.5 Read Path Changes

Current read path behavior differs by mode:
- **Time-based indices**: Jaeger computes exactly which indices to query based on the query's time range (e.g., only `jaeger-span-2024-06-17` and `jaeger-span-2024-06-18`). This is client-side pruning — very efficient, only relevant indices are touched.
- **Alias mode (rollover/ILM)**: Jaeger queries the single `jaeger-span-read` alias. ES/OpenSearch must check all indices behind the alias. Time range filters in the query body help, but there is no index-level pruning.

With data streams: query the data stream name `jaeger.spans` directly. ES/OpenSearch automatically fans out across all backing indices. Because backing indices are inherently time-ordered (each rollover creates a new one with a later time range), ES/OpenSearch can perform **index-level shard filtering** — it knows the min/max `@timestamp` of each backing index and skips those whose time range doesn't overlap the query. This gives data streams similar pruning efficiency to time-based indices, without Jaeger needing to compute index names client-side.

During migration (dual-lookup mode): query both the data stream and legacy indices. See §4.

### 3.6 Lifecycle Policy

Ship a default ISM/ILM policy as a static JSON resource embedded in the binary. Jaeger creates it on startup if it does not already exist (idempotent PUT). The policy is intentionally minimal.

Default policy (OpenSearch ISM):

An ISM policy is a state machine. State names (e.g., `hot`, `delete`) are arbitrary labels. Each state has **actions** (executed when an index enters the state) and **transitions** (conditions that move the index to the next state). The system periodically evaluates transition conditions on each managed index.

```json
{
  "policy": {
    "description": "Jaeger span data stream rollover and retention",
    "default_state": "hot",
    "states": [
      {
        "name": "hot",
        "actions": [
          {
            "rollover": {
              "min_primary_shard_size": "50gb",
              "min_doc_count": 200000000,
              "min_index_age": "5d"
            }
          }
        ],
        "transitions": [
          {
            "state_name": "delete",
            "conditions": {
              "min_index_age": "7d"
            }
          }
        ]
      },
      {
        "name": "delete",
        "actions": [
          {
            "delete": {}
          }
        ]
      }
    ],
    "ism_template": [
      {
        "index_patterns": [
          "jaeger.spans"
        ],
        "priority": 100
      }
    ]
  }
}
```

Reading this policy:
1. A new backing index starts in the `hot` state.
2. While in `hot`: the `rollover` action triggers when the primary shard exceeds 50GB or 200M documents, **or** when the index reaches 5 days old — whichever comes first. This creates a new backing index (which becomes the new write target).
3. The `transitions` condition is evaluated periodically: once the index is older than 7 days, it moves to the `delete` state.
4. Upon entering `delete`: the `delete` action removes the index.

The `ism_template` field auto-attaches this policy to any new index matching the pattern `jaeger.spans` (i.e., all backing indices of the data stream).

Key design choices:
- **`min_index_age: 5d` rollover trigger**: Ensures rollover happens well before the 7-day delete transition fires. Without a time-based trigger, a low-volume deployment that never hits the size/doc-count triggers would have ISM attempt to delete the only backing index — causing errors (data streams require at least one backing index). The 2-day buffer between rollover (5d) and deletion (7d) avoids race conditions from ISM evaluation timing.
- **7-day default retention**: Conservative default; users override via their own policy or `@custom` template.
- **No warm/cold phases**: These are deployment-specific (require dedicated node pools). Users add them via custom policies.
- **Idempotent creation**: If a policy with the same name already exists, Jaeger does NOT overwrite it. This respects user customizations.

Jaeger also supports a config option to point to a user-provided policy JSON file:

```yaml
es:
  data_stream:
    policy_file: "/etc/jaeger/my-policy.json"  # optional, overrides default policy
```

When set, Jaeger reads this file and uses its contents instead of the built-in default when creating the policy. This is simpler than requiring users to create a separate k8s job or script just to POST a policy to ES/OpenSearch before Jaeger starts.

The creation remains idempotent — if a policy with the same name already exists, Jaeger does not overwrite it. Users who want to update an existing policy must do so via the ES/OpenSearch API directly (or delete and restart).

### 3.7 Configuration

Recommendation: Replace the current tangle of boolean flags (`use_aliases`, `use_ilm`, `create_mappings`) with a one-of configuration structure under `indices.<type>.rotation`:

```yaml
elasticsearch:
  indices:
    index_prefix: "prod"
    spans:
      shards: 5
      replicas: 1
      rotation:
        # Exactly one of the following must be set.
        # Default (when rotation section is omitted entirely): periodic with date_layout "2006-01-02"
        periodic:
          date_layout: "2006-01-02"      # "2006-01-02-15" for hourly
        # manual_rollover:
        #   read_alias: ""               # optional custom alias override
        #   write_alias: ""              # optional custom alias override
        # auto_rollover:
        #   read_alias: ""               # optional custom alias override
        #   write_alias: ""              # optional custom alias override
        #   policy_name: "jaeger-ilm-policy"
        # data_stream:
        #   policy_name: "jaeger-spans-policy"
        #   policy_file: "/etc/jaeger/policy.json"  # optional
        #   read_alias: "jaeger.spans-read"         # optional, for migration
    services:
      shards: 5
      replicas: 1
      rotation:
        periodic:
          date_layout: "2006-01-02"
    dependencies:
      shards: 5
      replicas: 1
      rotation:
        periodic:
          date_layout: "2006-01-02"
```

The rotation strategy is configured **per index type**. This is necessary because services and dependencies cannot use data streams (they require upserts), so they remain on `periodic` even when spans use `data_stream`.

#### Config Structure in Go

Each rotation variant is a separate struct, composed using `configoptional.Optional` with a default:

```go
type Rotation struct {
    Periodic       configoptional.Optional[PeriodicRotation]       `mapstructure:"periodic"`
    ManualRollover configoptional.Optional[ManualRolloverRotation] `mapstructure:"manual_rollover"`
    AutoRollover   configoptional.Optional[AutoRolloverRotation]   `mapstructure:"auto_rollover"`
    DataStream     configoptional.Optional[DataStreamRotation]     `mapstructure:"data_stream"`
}

type PeriodicRotation struct {
    DateLayout string `mapstructure:"date_layout"`
}

type ManualRolloverRotation struct {
    ReadAlias  string `mapstructure:"read_alias"`
    WriteAlias string `mapstructure:"write_alias"`
}

type AutoRolloverRotation struct {
    ReadAlias  string `mapstructure:"read_alias"`
    WriteAlias string `mapstructure:"write_alias"`
    PolicyName string `mapstructure:"policy_name"`
}

type DataStreamRotation struct {
    PolicyName string `mapstructure:"policy_name"`
    PolicyFile string `mapstructure:"policy_file"`
    ReadAlias  string `mapstructure:"read_alias"`
}
```

Validation rejects configs where more than one variant is set (or where an unsupported rotation is used for services/dependencies).

#### Mapping to Existing Behavior

| `rotation` variant | Equivalent legacy flags | Behavior |
|--------------------|-------------------------|----------|
| `periodic` (default) | `use_aliases: false`, `use_ilm: false` | Daily/hourly indices, Jaeger creates templates on startup |
| `manual_rollover` | `use_aliases: true`, `use_ilm: false` | Numbered indices via aliases, requires `jaeger-es-rollover init` |
| `auto_rollover` | `use_aliases: true`, `use_ilm: true`, `create_mappings: false` | Rollover managed by ILM/ISM, requires `jaeger-es-rollover init` |
| `data_stream` | N/A (new) | Data streams, no external tooling needed |

#### Backward Compatibility with Legacy Flags

The old boolean flags (`use_aliases`, `use_ilm`, `create_mappings`) remain functional for backward compatibility but are deprecated. On startup, Jaeger resolves them to the equivalent `rotation` variant internally. If both the new `rotation` section and any legacy flag are set simultaneously, Jaeger fails startup with a validation error directing the user to remove the legacy flags.

Note: The legacy `date_layout` field (currently at the top level of each index type) moves into `rotation.periodic`. When the legacy field is present without a `rotation` section, it is treated as `rotation.periodic.date_layout` for backward compatibility.

This eliminates the class of misconfiguration bugs where users set contradictory flag combinations (e.g., `use_ilm: true` without `use_aliases: true`).

#### Strategy Interface (Implementation)

The code currently uses function closures (`spanAndServiceIndexFn`, `TimeRangeIndexFn`) selected at initialization time based on boolean flags. With four strategies and compositions (remote cluster prefixes, debug logging), this becomes harder to follow.

Recommendation: Replace closures with a `RotationStrategy` interface:

```go
// RotationStrategy encapsulates all index-management behavior for a given rotation mode.
type RotationStrategy interface {
    // WriteTarget returns the index/alias/data-stream name to write to.
    WriteTarget(spanTime time.Time) string
    // ReadTargets returns the index names to query for a given time range.
    ReadTargets(indexPrefix string, start, end time.Time) []string
    // CreateTemplates creates or updates index templates on startup.
    CreateTemplates(ctx context.Context, client es.Client) error
    // OpType returns the bulk operation type: "index" (legacy) or "create" (data streams).
    OpType() string
}
```

Each strategy implements this interface. Cross-cutting concerns (remote read clusters, debug logging) are decorators:

```go
// RemoteClusterStrategy wraps a RotationStrategy to add cross-cluster read indices.
type RemoteClusterStrategy struct {
    inner    RotationStrategy
    clusters []string
}

func (r *RemoteClusterStrategy) ReadTargets(prefix string, start, end time.Time) []string {
    targets := r.inner.ReadTargets(prefix, start, end)
    for _, t := range targets {
        for _, cluster := range r.clusters {
            targets = append(targets, cluster+":"+t)
        }
    }
    return targets
}
```

The factory creates the concrete strategy from config and passes it to the reader/writer. No boolean flags threaded through multiple files, no branching in the hot path. Each strategy is self-contained and independently testable.

### 3.8 OpenSearch vs. Elasticsearch

Both support data streams with nearly identical APIs. The differences:

| Aspect | OpenSearch | Elasticsearch |
|--------|-----------|---------------|
| Lifecycle policy | ISM (`_plugins/_ism/policies/`) | ILM (`_ilm/policy/`) |
| Policy structure | States + transitions | Phases + actions |
| Min version | 2.0 | 7.9 |
| Policy attachment | `ism_template` in policy | `index.lifecycle.name` in template |

Jaeger already detects OpenSearch vs. Elasticsearch at startup (via ping endpoint tagline). The lifecycle policy creation code branches on this detection:

```
if OpenSearch:
    PUT _plugins/_ism/policies/<policy-name>  (ISM format)
else:
    PUT _ilm/policy/<policy-name>  (ILM format)
    add index.lifecycle.name to index template settings
```

Both ES and OpenSearch are supported from the start. The data stream creation, write, and read paths are identical — only the lifecycle policy creation differs (ISM API for OpenSearch, ILM API for ES). CI integration tests run against both backends.

---

## 4. Migration & Backward Compatibility

This is the most critical section. Existing users have data in one of four modes:

| Mode | Indices | Aliases |
|------|---------|---------|
| 1. Time-based (default) | `jaeger-span-2024-06-18` | None |
| 2. Time-based + custom prefix | `prod-jaeger-span-2024-06-18` | None |
| 3. Manual rollover | `jaeger-span-000001` | `jaeger-span-read`, `jaeger-span-write` |
| 4. Rollover + ILM | `jaeger-span-000001` | `jaeger-span-read`, `jaeger-span-write` |

### 4.1 Migration Strategy: Read Alias

When spans use `rotation.data_stream`:
- **Writes** go exclusively to the data stream (`jaeger.spans`)
- **Reads** go to a configurable read target (default: the data stream name itself)

By default, Jaeger reads from the data stream name (`jaeger.spans`). For users migrating from a legacy strategy who still have old indices containing data, the `data_stream.read_alias` config option overrides the read target:

```yaml
elasticsearch:
  indices:
    spans:
      rotation:
        data_stream:
          policy_name: "jaeger-spans-policy"
          read_alias: "jaeger.spans-read"  # optional, defaults to the data stream name
```

When `read_alias` is set, Jaeger reads from that alias instead of the data stream name directly. This allows users to set up a unified alias spanning both the data stream and legacy indices, so that queries return results from both during the migration period.

**Jaeger does not create or manage this alias** — it is the user's responsibility to set it up as part of their migration procedure. See the migration instructions below.

#### Migration Instructions

**Important (OpenSearch)**: Data streams do not support aliases via the standard `_aliases` API in OpenSearch. Instead, the alias must be defined in the data stream's **index template** (see Appendix A for experimental verification on OpenSearch 3.7.0). The template-defined alias is automatically applied to all backing indices, including new ones created by rollover.

**Note (Elasticsearch)**: Elasticsearch 7.9+ supports data stream aliases via the `_aliases` API directly (the template `aliases` field is ignored for data stream backing indices on ES). On Elasticsearch, use the `_aliases` API to create a data stream alias and add legacy indices to it. The steps below focus on the OpenSearch approach; ES users should substitute the `_aliases` API call in place of the `@custom` template.

**Step 1**: Pre-create the `@custom` component template with the read alias. This **must be done before the first write** to the data stream — index templates are not applied retroactively to existing backing indices, so the alias will only appear on backing indices created after the template is in place.

```json
PUT _component_template/jaeger.spans@custom
{
  "template": {
    "aliases": {
      "jaeger.spans-read": {}
    }
  }
}
```

> **Note:** If you have configured `index_prefix` (e.g., `"prod"`), you must include it in all template and alias names:
> ```json
> PUT _component_template/prod.jaeger.spans@custom
> {
>   "template": {
>     "aliases": {
>       "prod.jaeger.spans-read": {}
>     }
>   }
> }
> ```

**Step 2**: Switch Jaeger to data-stream mode. The first span write will create the data stream and its first backing index, which will have the alias from the `@custom` template.

**Step 3**: Add legacy indices to the same alias:

```json
POST /_aliases
{
  "actions": [
    { "add": { "index": "jaeger-span-*", "alias": "jaeger.spans-read" } }
  ]
}
```

For users migrating from rollover mode (modes 3-4), `jaeger-span-*` matches the numbered indices. For time-based mode (modes 1-2) with a custom prefix, use `<prefix>-jaeger-span-*`.

**Step 4**: Configure Jaeger to read from the alias:

```yaml
elasticsearch:
  indices:
    spans:
      rotation:
        data_stream:
          read_alias: "jaeger.spans-read"
```

This approach:
- Requires no special dual-query logic in Jaeger's read path — it's a single query target.
- Lets legacy indices expire naturally via their existing TTL mechanisms (`jaeger-es-index-cleaner` or ILM policy). As indices are deleted, they automatically disappear from the alias.
- Takes advantage of ES/OpenSearch index-level shard filtering across all members of the alias.
- Keeps alias lifecycle management out of Jaeger's production code — users have full control over their migration timeline.

#### End of Migration

Once all legacy indices have aged out and been deleted by their existing cleanup mechanisms, no explicit action is needed — the alias simply stops matching any legacy indices. Users can then:
1. Remove the `read_alias` config (Jaeger reverts to reading from the data stream name directly).
2. Optionally remove the `@custom` component template and the now-empty alias.

### 4.2 Fresh Installations

For new deployments with no legacy data, no migration steps are needed. Simply set `index_management: data_stream` and leave `read_alias` unset. Jaeger reads and writes directly to/from the data stream.

### 4.3 Rollback Path

If a user enables data streams and encounters problems, the rollback strategy depends on which mode they are rolling back to:

#### Rolling back to Manual Rollover or Auto Rollover

These modes already use a read alias (`jaeger-span-read`). The same forward-migration technique works in reverse — add the data stream to the existing read alias:

1. Set `index_management` back to `manual_rollover` or `auto_rollover`.
2. Add the data stream's backing indices to the existing read alias. Since the data stream's index template can include an alias (see Appendix A), add `jaeger-span-read` to the data stream's `@custom` component template:
   ```json
   PUT _component_template/jaeger.spans@custom
   {
     "template": {
       "aliases": {
         "jaeger-span-read": {}
       }
     }
   }
   ```
3. Jaeger in rollover mode already reads from `jaeger-span-read`, so data from both the data stream and rollover indices is visible immediately.
4. Once the data stream's ISM policy deletes old backing indices (or the user manually deletes the data stream), the `@custom` template can be removed.

#### Rolling back to Time-based Indices

There is no clean solution for time-based mode. Jaeger in time-based mode computes specific index names from the query time range (e.g., `jaeger-span-2024-06-18`) — there is no alias or wildcard query that would naturally include data stream backing indices.

Possible workarounds (all have significant limitations):
- **Re-index**: Use the `_reindex` API to copy data from the data stream into time-based indices matching the expected naming pattern. This is operationally expensive for large volumes.
- **Accept data loss for the rollback window**: If the data stream was only active for a short period (e.g., one day), the data from that period becomes invisible to Jaeger queries after rollback. It remains in ES/OpenSearch and is queryable directly, but Jaeger won't find it.

Recommendation: Users on time-based mode who want a safe rollback path should first migrate to `auto_rollover` (which provides alias-based reads) before attempting the switch to `data_stream`. This gives them the alias-based rollback mechanism described above.

### 4.4 Impact on External Tools

| Tool | Impact | Migration |
|------|--------|-----------|
| `jaeger-es-rollover` | No longer needed | Deprecate; document that data streams handle initialization and rollover |
| `jaeger-es-index-cleaner` | No longer needed for spans | Deprecate for spans (ISM/ILM handles retention). Can be retained for services index cleanup, but given trivial data volume this is optional (see §2.1) |
| Kibana/Grafana index patterns | `jaeger-span-*` won't match new data | Users update patterns to include `jaeger.spans` or use a wildcard that covers both |
| Custom Terraform/Helm templates | May conflict | Document that Jaeger creates templates on startup; external templates should use `@custom` pattern |

### 4.5 Multi-Tenancy / Index Prefix

Current behavior: `index_prefix: "prod"` produces `prod-jaeger-span-*`.

With data streams: the prefix applies to the data stream name: `prod.jaeger.spans`.

The component templates are also prefixed with dot-notation: `prod.jaeger.spans@mappings`, `prod.jaeger.spans@settings`.

The lifecycle policy name is fully user-configured via `data_stream.policy_name` (default: `jaeger-spans-policy`). It is NOT auto-derived from `index_prefix`. In multi-tenant setups, operators should configure distinct policy names per tenant (e.g., `policy_name: "prod-jaeger-spans-policy"`).

---

## 5. Open Questions

### Q1: Should services/dependencies eventually move to data streams?

Services index is a deduplication cache (service name → exists). Dependencies are batch-computed aggregates. Neither is append-only. Data streams are not suitable unless the storage model changes fundamentally.

**Recommendation**: No. Keep them as standard indices. Consider automating their lifecycle with ISM/ILM separately (not in this RFC's scope).

### Q2: Feature gate or config flag?

Should data streams be behind a feature gate (enabled by default after N releases) or a permanent config flag?

Recommendation: Permanent enum value in `index_management`. Feature gates imply eventual removal of the old path. We should support legacy modes indefinitely for users who cannot use data streams (e.g., ES versions < 7.9, restrictive ES policies).

### Q3: Minimum supported version

Data streams require ES 7.9+ or OpenSearch 2.0+. Jaeger currently supports ES 7.x and 8.x, OpenSearch 1.x, 2.x, 3.x.

Recommendation: Require OpenSearch 2.0+ or ES 7.9+ for `index_management: data_stream`. Fail startup with a clear error if the detected version is too old. OpenSearch 1.x users must upgrade before enabling data streams.

### Q4: What happens to `CreateIndexTemplates` flag?

Recommendation: When `index_management: data_stream`, Jaeger ALWAYS creates the composable index template (the internal `create_mappings` flag is set to `true`). Rationale: data stream templates are cheap to create, idempotent, and the `@custom` pattern ensures user customizations are never overwritten.

For legacy mode, `create_mappings` continues to work as before. No deprecation needed — it's orthogonal.

### Q5: Naming the lifecycle policy

Should the policy name be configurable or fixed?

**Recommendation**: Configurable with a sensible default (`jaeger-spans-policy`). Users in multi-tenant setups need distinct policy names per prefix.

---

## 6. Implementation Plan

Design principles for phasing:
- **API changes first**: Config and interface changes that affect all strategies ship early, giving users time to adopt the new config before data streams land.
- **Incremental delivery**: Each phase is independently shippable and valuable without the next.
- **CI from the start**: Every phase includes integration tests that validate the change against real ES/OpenSearch instances.

### Phase 1: Config Refactoring & Strategy Interface

Introduce the `rotation` one-of config structure and `RotationStrategy` interface. No new functionality — existing strategies are re-expressed through the new abstractions.

1. Introduce `Rotation` config struct with `Periodic`, `ManualRollover`, `AutoRollover`, `DataStream` variants (§3.7)
2. Implement backward-compat parsing: legacy flags (`use_aliases`, `use_ilm`, `create_mappings`, `date_layout`) map to the corresponding `rotation` variant; error if both old and new are set
3. Introduce `RotationStrategy` interface (§3.7); implement `PeriodicStrategy`, `ManualRolloverStrategy`, `AutoRolloverStrategy`
4. Refactor writer/reader/factory to accept `RotationStrategy` instead of boolean flags
5. Add `DataStreamRotation` config variant (validation accepts it, but factory rejects with "not yet implemented")
6. **CI**: All existing ES/OpenSearch integration tests must pass unchanged (proves the refactor is behavior-preserving)

Deliverable: cleaner config, no spaghetti branching, `data_stream` visible in config schema (but not yet functional).

### Phase 2: Data Stream Write Path

Make data streams functional for writes. Reads still go to the data stream name directly (no migration alias yet).

7. Add `@timestamp` field (date_nanos) to span document at write time
8. Implement `DataStreamStrategy.CreateTemplates()`: composable index template + component templates (§3.2)
9. Implement `DataStreamStrategy.WriteTarget()`: return data stream name
10. Implement `DataStreamStrategy.OpType()`: return `"create"`
11. Implement ISM policy creation for OpenSearch, ILM for Elasticsearch (§3.6)
12. Implement `DataStreamStrategy.ReadTargets()`: return data stream name (no migration alias yet)
13. **CI**: Integration test — write spans via data stream, read them back, verify end-to-end on both OS and ES

Deliverable: `rotation.data_stream` is fully functional for fresh installations (no legacy data).

### Phase 3: Migration Support

Enable the `read_alias` option for users with existing data.

14. Add `read_alias` field to `DataStreamRotation` config
15. Implement `DataStreamStrategy.ReadTargets()` override: when `read_alias` is set, read from it instead of data stream name
16. **CI**: Integration test — write legacy indices, switch to data stream mode with `read_alias`, verify queries return data from both sources
17. Document migration procedure for all four legacy modes (§4.1)

Deliverable: existing users can safely migrate to data streams with zero data loss during transition.

### Phase 4: Documentation & Deprecation

18. Deprecate `jaeger-es-rollover` tool (keep functional, log deprecation warning when invoked)
19. Deprecate `jaeger-es-index-cleaner` for spans
20. Deprecate legacy boolean config flags (`use_aliases`, `use_ilm`, `create_mappings`) — log warning at startup when used
21. Publish data stream configuration guide in Jaeger docs

### Phase 5: Future (not in initial implementation)

- Graduate from experimental to stable based on community feedback
- Consider making `data_stream` the default rotation for spans in new installations
- In-process index cleaner for services index (§2.1)
- Deprecation of legacy strategies only after extended period and with clear migration tooling (if ever)

---

## 7. References

- [Issue #4708: Support Elasticsearch/OpenSearch data stream](https://github.com/jaegertracing/jaeger/issues/4708)
- [PR #7768: Add Elasticsearch data stream support](https://github.com/jaegertracing/jaeger/pull/7768) (closed)
- [PR #7974: Draft ADR](https://github.com/jaegertracing/jaeger/pull/7974) (superseded by this RFC)
- [Issue #6458: Upgrade Storage Backends to V2 Storage API](https://github.com/jaegertracing/jaeger/issues/6458)
- [Issue #7612: Replace olivere/elastic driver](https://github.com/jaegertracing/jaeger/issues/7612)
- [Issue #8078: ES/OS backend ignore scope and link attributes](https://github.com/jaegertracing/jaeger/issues/8078)
- [Elasticsearch Data Streams docs](https://www.elastic.co/guide/en/elasticsearch/reference/current/data-streams.html)
- [OpenSearch Data Streams docs](https://docs.opensearch.org/latest/im-plugin/data-streams/)
- [OpenSearch ISM docs](https://docs.opensearch.org/latest/im-plugin/ism/index/)
- [ES `@custom` component template pattern](https://www.elastic.co/docs/manage-data/lifecycle/index-lifecycle-management/tutorial-customize-built-in-policies)

---

## Appendix A: Alias + Data Stream Experiment (OpenSearch 3.7.0)

**Date**: 2026-06-18
**Environment**: OpenSearch 3.7.0 (docker image `opensearchproject/opensearch:3.7.0`), single node, security disabled.

### Goal

Verify whether a single alias can span both a data stream and regular (legacy) indices, enabling unified reads during migration.

### Findings

| Approach | Result |
|----------|--------|
| `_aliases` API with data stream name as `"index"` | **Fails**: `index_not_found_exception` — the `_aliases` API does not recognize data stream names. |
| `_aliases` API with backing index (`.ds-jaeger.spans-000001`) | **Fails**: `illegal_argument_exception` — "Data streams and their backing indices don't support aliases." |
| Alias defined in the data stream's **index template** | **Works**: alias is automatically applied to every backing index, including new ones created by rollover. |
| Adding regular indices to that same alias via `_aliases` API | **Works**: the alias then spans both data stream backing indices and legacy indices. |
| Multi-target search (`GET /jaeger.spans,jaeger-span-*/_search`) | **Works**: returns results from both targets without any alias. |

### Reproduction Steps

```bash
# 1. Start OpenSearch 3
docker compose -f docker-compose/opensearch/v3/docker-compose.yml up -d

# 2. Create composable index template for data stream, with an alias
curl -X PUT "http://localhost:9200/_index_template/jaeger-spans-template" \
  -H 'Content-Type: application/json' -d '{
  "index_patterns": ["jaeger.spans"],
  "data_stream": {},
  "template": {
    "mappings": {
      "properties": {
        "@timestamp": { "type": "date_nanos" },
        "traceID": { "type": "keyword" },
        "spanID": { "type": "keyword" }
      }
    },
    "aliases": {
      "jaeger.spans-read": {}
    }
  }
}'

# 3. Create the data stream by writing a document
curl -X POST "http://localhost:9200/jaeger.spans/_doc" \
  -H 'Content-Type: application/json' -d '{
  "@timestamp": "2024-06-18T10:00:00.000000000Z",
  "traceID": "abc123",
  "spanID": "span001"
}'

# 4. Create a legacy index with a document
curl -X PUT "http://localhost:9200/jaeger-span-2024-06-17" \
  -H 'Content-Type: application/json' -d '{
  "mappings": {
    "properties": {
      "@timestamp": { "type": "date_nanos" },
      "traceID": { "type": "keyword" },
      "spanID": { "type": "keyword" }
    }
  }
}'
curl -X POST "http://localhost:9200/jaeger-span-2024-06-17/_doc" \
  -H 'Content-Type: application/json' -d '{
  "@timestamp": "2024-06-17T15:00:00.000000000Z",
  "traceID": "def456",
  "spanID": "span002"
}'

# 5. Add legacy index to the same alias
curl -X POST "http://localhost:9200/_aliases" \
  -H 'Content-Type: application/json' -d '{
  "actions": [
    { "add": { "index": "jaeger-span-2024-06-17", "alias": "jaeger.spans-read" } }
  ]
}'

# 6. Verify: search the alias returns docs from both
curl "http://localhost:9200/jaeger.spans-read/_search" \
  -H 'Content-Type: application/json' -d '{"query":{"match_all":{}}}'
# Result: 2 hits — one from .ds-jaeger.spans-000001, one from jaeger-span-2024-06-17

# 7. Verify: alias survives rollover
curl -X POST "http://localhost:9200/jaeger.spans/_rollover"
curl -X POST "http://localhost:9200/jaeger.spans/_doc" \
  -H 'Content-Type: application/json' -d '{
  "@timestamp": "2024-06-19T08:00:00Z",
  "traceID": "ghi789",
  "spanID": "span003"
}'
curl "http://localhost:9200/jaeger.spans-read/_search" \
  -H 'Content-Type: application/json' -d '{"query":{"match_all":{}}}'
# Result: 3 hits — two from data stream (pre/post rollover), one from legacy index
```

### Conclusion

The migration alias approach is viable but requires the alias to be defined in the **index template** (not via the `_aliases` API on the data stream itself). The recommended mechanism is a `@custom` component template created by the user as part of their migration procedure. Jaeger's role is limited to accepting a `read_alias` config override — it does not need to manage the alias itself.
