# RFC 0007: Synchronous Elasticsearch/OpenSearch Writes

- **Status:** Draft
- **Author:** Yuri Shkuro
- **Created:** 2026-07-06
- **Last Updated:** 2026-07-06
- **Issue:** [#8476](https://github.com/jaegertracing/jaeger/issues/8476)
- **Related:** [RFC 0004 Data Streams](./0004-elasticsearch-data-streams.md) · [RFC 0006 Unified ES Client](./0006-unified-elasticsearch-client.md) · [#7612](https://github.com/jaegertracing/jaeger/issues/7612) · [#2192](https://github.com/jaegertracing/jaeger/issues/2192) · PRs [#8281](https://github.com/jaegertracing/jaeger/pull/8281), [#8651](https://github.com/jaegertracing/jaeger/pull/8651)

---

## Abstract

Jaeger's Elasticsearch/OpenSearch (ES/OS) trace writer enqueues spans into an **asynchronous** client-side bulk buffer (`olivere/elastic`'s `BulkProcessor`) and returns success to its caller **before** the data is durable in the backend. This silently violates the `tracestore.Writer` contract — `WriteTraces` returns `nil` even when the eventual bulk flush fails — and, on the Kafka ingester path, causes Jaeger to **commit Kafka offsets for data that was never persisted**, i.e. silent trace loss on backend outage or overload ([#8476](https://github.com/jaegertracing/jaeger/issues/8476)).

This RFC establishes the facts that shape the fix — a single `_bulk` HTTP request to ES/OS is already **synchronous and durable**; the asynchrony is entirely a client-side artifact — and shows that the ClickHouse-style *server-side* batched-insert model the reporter asked about **does not exist** in ES/OS. It then proposes a **synchronous batch-write mode**: preserve the OTLP batch through the pipeline and issue **one synchronous, size-bounded `_bulk` request per batch**, checking item-level results and returning a real error. This restores the writer contract, gives Kafka correct at-least-once behavior, and turns the pipeline's existing batching (Kafka producer sizing + the OTel batch processor) into the sole, sufficient batching layer — removing the redundant client-side buffer.

This is a design exploration, not a committed decision. It is scoped to interact cleanly with the bounded bulk indexer already planned in [RFC 0006 M6](./0006-unified-elasticsearch-client.md#8-migration-plan).

---

## 1. Motivation

### 1.1 The contract violation

The v2 writer interface is explicit ([`internal/storage/v2/api/tracestore/writer.go`](../../internal/storage/v2/api/tracestore/writer.go)):

```go
// WriteTraces ... if any of the spans fail to be written an error is returned.
// Compatible with OTLP Exporter API.
WriteTraces(ctx context.Context, td ptrace.Traces) error
```

The ES implementation does not honor it. `TraceWriter.WriteTraces` converts the batch and, per span, calls the fire-and-forget `SpanWriter.WriteSpan`, then unconditionally returns `nil` ([`internal/storage/v2/elasticsearch/tracestore/writer.go`](../../internal/storage/v2/elasticsearch/tracestore/writer.go)):

```go
func (t *TraceWriter) WriteTraces(_ context.Context, td ptrace.Traces) error {
    dbSpans := ToDBModel(td)
    for i := range dbSpans {
        span := &dbSpans[i]
        t.spanWriter.WriteSpan(model.EpochMicrosecondsAsTime(span.StartTime), span)
    }
    return nil
}
```

`WriteSpan` ultimately calls `client.Index()…BodyJson(&jsonSpan).Add()` ([`core/writer.go`](../../internal/storage/v2/elasticsearch/tracestore/core/writer.go)). `Add()` merely **enqueues** into the shared `elastic.BulkProcessor` ([`wrapper/wrapper.go`](../../internal/storage/elasticsearch/wrapper/wrapper.go)) and returns. The buffer flushes later on its own triggers, and bulk failures surface only in the processor's `After` callback ([`clientbuilder/builder.go`](../../internal/storage/elasticsearch/clientbuilder/builder.go)), which logs and updates metrics but **cannot influence the already-returned `WriteTraces`**.

Cassandra and ClickHouse v2 writers honor the same contract synchronously (they `errors.Join` per-span failures / return `Append`/`Send` errors). **ES is the outlier.**

### 1.2 Why it matters: Kafka silent data loss

Jaeger v2 is an OpenTelemetry Collector assembly. The ingester pipeline is:

```
kafka receiver ──▶ batch processor ──▶ jaeger_storage_exporter ──▶ WriteTraces
```

(`config-kafka-ingester.yaml`). The storage exporter wraps `WriteTraces` in `exporterhelper` with **no sending queue and retry disabled by default** ([`storageexporter/factory.go`](../../cmd/jaeger/internal/exporters/storageexporter/factory.go), [`config.go`](../../cmd/jaeger/internal/exporters/storageexporter/config.go)), so `ConsumeTraces → pushTraces → WriteTraces` is a straight synchronous call — **the only asynchronous hop is the `BulkProcessor` buffer inside the writer.**

The upstream OTel `kafkareceiver` commits a message's offset **when `ConsumeTraces` returns `nil`**. Because `WriteTraces` returns `nil` at *enqueue* time, the sequence is:

1. Ingester reads a Kafka message (a batch of spans).
2. `WriteTraces` enqueues them and returns `nil`.
3. Receiver marks the offset committed.
4. **Later**, the `BulkProcessor` flushes — and if ES is down, overloaded, or rejects the mapping, the batch is lost.

The offset is already gone. This converts any transient backend problem into **permanent, invisible trace loss**, and defeats the entire point of buffering traces in Kafka. The same gap removes backpressure: the pipeline cannot slow down or apply retry because it never learns the write failed.

### 1.3 Existing attempts

- **[PR #8281](https://github.com/jaegertracing/jaeger/pull/8281)** (closed) — replaced the async processor in the v2 path with a synchronous `client.Bulk().Do(ctx)` plus `resp.Errors` checking. The right direction, closed pending a design decision.
- **[PR #8651](https://github.com/jaegertracing/jaeger/pull/8651)** (open, draft) — "sticky error": record the last async bulk error behind an `atomic.Pointer` and return it on the *next* `WriteTraces`. A minimal patch that closes the contract gap in the loosest sense but attributes the error to the wrong batch and still commits the failing batch's offset.

This RFC exists to pick the direction those PRs were waiting on.

---

## 2. Background: how ES/OS writes actually work

Three facts, each load-bearing for the design and each easy to get wrong.

### 2.1 A `_bulk` request is synchronous and durable

A single `_bulk` HTTP request is **not** asynchronous on the server. Under the default `index.translog.durability: request`, ES/OS `fsync` and commit the translog on the primary **and every allocated replica** before responding, so a `200` means *"all acknowledged writes have been committed to disk"* ([ES translog settings](https://www.elastic.co/docs/reference/elasticsearch/index-settings/translog)). OpenSearch is identical. The `async` durability mode only changes *when* the fsync happens (every `sync_interval`, default 5s) — it does not make the request itself asynchronous, and is a separate, orthogonal knob.

**Implication:** durability is a property Jaeger already gets for free from a single bulk round-trip. The async behavior Jaeger exhibits is **purely client-side** — an artifact of the `BulkProcessor`, not of ES.

### 2.2 Durable ≠ searchable (and that's fine here)

Search visibility is a *separate* axis governed by **refresh** (`index.refresh_interval`, default `1s`; the `refresh` bulk parameter defaults to `false`). A normal bulk `200` means the docs are **durable**, not necessarily **searchable** yet ([refresh parameter](https://www.elastic.co/docs/reference/elasticsearch/rest-apis/refresh-parameter)).

This distinction does **not** affect this RFC: the writer contract and Kafka at-least-once only require **durability** (no acknowledged-but-lost data). We deliberately do **not** propose `refresh=wait_for`/`true` on writes — forcing refresh would wreck indexing throughput for no correctness benefit. Near-real-time search remains the existing ~1s behavior.

### 2.3 There is no server-side "async insert" in ES/OS

The reporter asked whether ES has a ClickHouse-style server-side batched insert — where the **server** coalesces many independent client requests into one flush and can block the client until that flush completes.

**ClickHouse has this.** `async_insert=1` writes incoming INSERTs into a **server-side** in-memory buffer flushed on thresholds (`async_insert_max_data_size` ≈100 MiB, `async_insert_busy_timeout_ms` ≈200 ms, `async_insert_max_query_number` =450), *"invisible to clients … merging insert traffic from multiple sources."* With `wait_for_async_insert=1` (default) the client's INSERT **blocks until the buffered batch is flushed to disk**, yielding a synchronous durability ack on top of server-side batching ([ClickHouse async inserts](https://clickhouse.com/docs/optimize/asynchronous-inserts)). This is exactly the model the reporter described.

**ES/OS have no equivalent.** There is no server-side mode that buffers across separate client requests with an optional wait-for-flush ack. The only batching primitives are:

- the `_bulk` API — batches many docs, but within *one* client request; no cross-request coalescing;
- client-side buffering helpers (olivere `BulkProcessor`, go-elasticsearch `esutil.BulkIndexer`, Logstash/Beats) — batching lives in the *client*;
- `translog.durability: async` — an fsync-timing knob, still per-request, not coalescing.

So the ClickHouse option is off the table for ES/OS. The equivalent has to be built where ES puts it — **at the client / pipeline** — which is precisely what this RFC does: let the pipeline (Kafka + batch processor) form the batch, and make the *client's* per-batch write synchronous. (The one place ES could "block until a batch fills" is the client-side buffer's `FlushInterval` — but that blocks *nothing* and acks *nothing*, which is the bug.)

### 2.4 The current async knobs

The `BulkProcessor` is configured from `bulk_processing` ([`config.go`](../../internal/storage/elasticsearch/config/config.go)); Jaeger defaults: `max_bytes` 5 MB, `max_actions` 1000, `flush_interval` 200 ms, `workers` 1. These shape *client-side* flushes. Note the processor has no hard pre-flush byte ceiling, which is the root of the unbounded-memory / `413 Request Entity Too Large` bug [#2192](https://github.com/jaegertracing/jaeger/issues/2192) — a size-bounded indexer is already planned in [RFC 0006](./0006-unified-elasticsearch-client.md).

---

## 3. Design options

Goal: `WriteTraces` returns a truthful error for the batch it was given, and (on Kafka) the offset commits only after that batch is durable — without collapsing throughput.

Criteria:
- **Contract** — does `WriteTraces` return real write errors?
- **Correct offset / at-least-once** — is the *failing* batch's Kafka offset withheld (backpressure/retry)?
- **Attribution** — is an error tied to the batch that caused it?
- **Throughput** — is bulk batching preserved?
- **Backpressure** — can the pipeline slow down under write pressure?
- **Complexity** — implementation cost.

Options as columns, criteria as rows. 🟢 good · 🟡 partial/caveated · 🔴 poor.

| Criterion | A. Async status quo | B. Sticky-error (#8651) | C. `Flush()` per call | **D. Sync batch write (rec.)** |
|---|:--:|:--:|:--:|:--:|
| Contract (real errors) | 🔴 always `nil` | 🟡 delayed, best-effort | 🟢 | 🟢 |
| Correct offset / at-least-once | 🔴 | 🔴¹ | 🟢 | 🟢 |
| Attribution | 🔴 | 🔴¹ | 🟢 | 🟢 |
| Throughput | 🟢 | 🟢 | 🔴² | 🟡³ |
| Backpressure | 🔴 | 🔴 | 🟡 | 🟢 |
| Complexity | 🟢 exists | 🟢 small | 🟢 small | 🟡 moderate |

Legend / footnotes:
- ¹ The error is recorded against a later call; the batch that actually failed has **already** had its offset committed. At-least-once is not restored.
- ² `BulkProcessor.Flush()` is a *global* synchronous drain of the shared buffer across all workers; calling it every `WriteTraces` serializes every caller onto one flush and destroys coalescing.
- ³ Efficiency now depends on the **pipeline** delivering appropriately sized batches (§4.2). With a well-sized batch it is one efficient bulk per batch; with a firehose of tiny batches it degrades to many small bulks (mitigated by upstream batching, which already exists).

**Option D is recommended.** It is the only option that restores both the contract *and* Kafka at-least-once with correct attribution, and it is the model the reporter proposed. C is rejected (throughput). B is a stop-gap that does not fix the data-loss bug for the failing batch. A is the bug.

---

## 4. Recommended design: synchronous batch write

### 4.1 One synchronous bulk per pipeline batch

Replace the per-span enqueue with a per-batch synchronous write:

1. `WriteTraces` converts the whole `ptrace.Traces` batch to `[]dbmodel.Span` (as today).
2. It assembles **one** `_bulk` body containing every span document **and** any service/operation documents the batch requires (§4.3), split into size-bounded sub-requests (§4.4).
3. It issues the bulk **synchronously** (`Bulk().Do(ctx)` on the current client, or `BulkWriter` in the RFC 0006 world — §5), inspects `resp.Errors` and each item's status, and **returns an error** if the transport failed or any item failed.
4. Only on `nil` does the exporter return success → the Kafka offset commits.

This requires the core writer to expose a batch, error-returning entry point (the `#8476` discussion sketched exactly this):

```go
// core.Writer
WriteSpans(ctx context.Context, spans []dbmodel.Span) error   // replaces fire-and-forget WriteSpan
```

`TraceWriter.WriteTraces` calls it once per batch and returns its error verbatim. The `SpanWriter`'s existing responsibilities (tag elevation, index/rotation target selection, `@timestamp` for data streams, service dedup cache) are unchanged — only the *sink* changes from "enqueue into shared buffer" to "append into this batch's bulk request."

### 4.2 The pipeline is the batching layer

With synchronous per-batch writes, batch **size** is what determines efficiency, and the pipeline already controls it:

- **Kafka path (primary beneficiary).** The collector's `kafka` exporter serializes each exported `ptrace.Traces` into one Kafka message, already shaped by the collector's `batch` processor. On the ingester, **one Kafka message → one synchronous bulk** is the natural, optimal mapping — the batch formed once at produce time is preserved end-to-end and written atomically. The ingester's own `batch` processor becomes optional: keep it to *re-shape* (coalesce small messages / split huge ones), or drop it to preserve the exact Kafka message boundary. This is the reporter's core insight — Kafka already did the batching; don't redo it in a fire-and-forget client buffer that also throws away the durability signal.
- **Collector path (OTLP/Jaeger receivers → storage directly).** Here batches arrive at whatever cadence receivers produce. The OTel `batch` processor (or the newer `exporterhelper` `QueueBatchConfig` batcher on the storage exporter) sizes them. Operators who want large bulks configure `send_batch_size`/`timeout` as they would for any exporter.

The redundant third buffer (the client-side `BulkProcessor`) is removed in sync mode. Batching is done **once**, in the pipeline, where it is observable and tunable — not hidden inside the storage client.

### 4.3 Service/operation documents

`WriteSpan` today also writes a `service:operation` dedup document per new (service, operation) pair, gated by an in-memory TTL cache ([`core/service_operation.go`](../../internal/storage/v2/elasticsearch/tracestore/core/service_operation.go)). In sync mode these documents are appended to the **same** bulk request as the spans, so they share the batch's durability and error signal. The cache is only updated **after** a successful bulk — otherwise a failed-then-retried batch could mark a service as written when it was not, silently dropping it. (This "update cache after durable write" ordering mirrors the write-path cache discipline in [RFC 0004 §3.9](./0004-elasticsearch-data-streams.md).)

### 4.4 Size bounding, retries, and duplicates

- **Size bound (`#2192`).** A single Kafka message can be large; a naive one-bulk-per-batch could exceed `http.max_content_length` (default 100 MB) and 413. The sync writer chunks a batch into sub-requests capped at `max_bytes`, issues them in sequence, and joins their errors. This gives the sync path the hard byte ceiling the async processor never had — closing [#2192](https://github.com/jaegertracing/jaeger/issues/2192) for this path directly.
- **Retry.** On a returned error the pipeline retries the *whole* batch (Kafka re-delivery, or `exporterhelper` retry). Item-level `429/503` (backpressure) therefore retry naturally. We do **not** silently swallow partial failures.
- **Duplicates / idempotency.** Jaeger sets no document `_id`, so retries can create duplicate span docs — but this is **already** the case today on any retry, and is tolerated (append-only, at-least-once; see [RFC 0004 §3.4](./0004-elasticsearch-data-streams.md)). Synchronous retry makes it *more visible*, not new. A deterministic `_id` (trace+span+start) with `op_type=create` would give at-least-once dedup and is cross-referenced as a future improvement in RFC 0004; out of scope here.

### 4.5 Configuration

Introduce an explicit write mode rather than overloading the async knobs:

```yaml
elasticsearch:
  write_mode: async   # async (default, legacy) | sync
  bulk_processing:     # applies to async mode; in sync mode only `max_bytes` is honored (chunk cap)
    max_bytes: 5000000
    max_actions: 1000
    flush_interval: 200ms
    workers: 1
```

- `async` (default): today's `BulkProcessor` behavior, unchanged — backward compatible.
- `sync`: the design above. `max_bytes` is reused as the per-request chunk ceiling (§4.4); `max_actions`, `flush_interval`, `workers` are inert (batching is the pipeline's job).

Rationale for opt-in first: sync mode changes throughput characteristics and depends on sane upstream batch sizing; making it default (especially for the Kafka ingester) is a follow-up once validated (§7 Q1).

---

## 5. Sequencing with RFC 0006 and RFC 0004

This work is small but touches the write path that [RFC 0006](./0006-unified-elasticsearch-client.md) is actively replacing:

- **RFC 0006 M6** introduces the owned `BulkWriter` and the bounded bulk indexer (fixing #2192). The synchronous write should be a **first-class method on `BulkWriter`** (e.g. `Bulk(ctx, items) (BulkResult, error)`), not bolted onto `olivere`. The cleanest sequencing is to **land the sync-mode plumbing (writer API `WriteSpans`, config, service-doc ordering) against the current client, then have M6 provide the synchronous, size-bounded bulk primitive** — or to fold the synchronous path into M6 directly. Either way, sync mode must **not** reintroduce a dependency on `olivere`'s async processor.
- **RFC 0004 (Data Streams)** already changes the write op-type (`index`→`create`) and adds `@timestamp`. The sync writer inherits both transparently — it writes whatever documents `SpanWriter` produces. The service-doc cache ordering (§4.3) reuses the same "cache after durable write" discipline data streams' trace-timestamp index needs.

There is no conflict; there is shared surface. The recommendation is to treat "synchronous write mode" as a property delivered **on top of** the RFC 0006 `BulkWriter`, with the contract/Kafka fix (this RFC) as the motivating requirement for that primitive.

---

## 6. Migration & backward compatibility

- **Default unchanged.** `write_mode: async` remains the default; existing deployments see no behavior change.
- **Opt-in.** Operators set `write_mode: sync` (recommended together with a tuned `batch` processor, or by removing it on the ingester to preserve Kafka message boundaries). Correctness improves (no acked-but-lost data); throughput becomes a function of batch size.
- **No schema/on-disk change.** Documents, indices, mappings, and read paths are identical. This is purely a write-*mechanism* change.
- **Rollback.** Flip back to `write_mode: async`. No data migration.

---

## 7. Open questions

**Q1 — Should `sync` become the default for the Kafka ingester?**
The ingester is where async writes cause real data loss, and where batches are already well-formed. A reasonable end state is `sync` default on the ingester, `async` default (or configurable) on the direct collector→storage path. Recommendation: ship opt-in, then default-on-for-ingester after validation.

**Q2 — Keep or drop the ingester `batch` processor by default?**
Preserving the Kafka message as the bulk unit (drop the processor) is the simplest, most predictable mapping. Re-shaping (keep it) helps when producers emit tiny messages. Recommendation: document both; keep the processor in the default config but note that removing it makes 1 message = 1 durable bulk.

**Q3 — Interface doc for `WriteTraces` async allowance.**
Regardless of mode, the `tracestore.Writer` doc comment should state explicitly whether an implementation may be asynchronous. Today the comment implies synchronous semantics that only Cassandra/ClickHouse honor. Recommendation: document that `WriteTraces` **must** return write errors synchronously; `async` mode is a deliberate, documented deviation for throughput at the cost of the guarantee.

**Q4 — Deterministic `_id` for dedup?**
Synchronous retries make duplicate spans more visible. Adopting a deterministic `_id` + `op_type=create` would give at-least-once dedup. Recommendation: keep out of scope; track with RFC 0004's identical open question to avoid double-deciding.

**Q5 — `exporterhelper` batcher vs. OTel `batch` processor.**
The newer `QueueBatchConfig` batcher on the storage exporter could replace the pipeline `batch` processor and colocate batching with the write. Worth evaluating but not required for correctness. Recommendation: defer.

---

## 8. Implementation plan

Each step is independently shippable and guarded by unit + ES/OS integration tests.

1. **Writer contract + core API.** Add `core.Writer.WriteSpans(ctx, []span) error`; keep `WriteSpan` for the async path. Update the `tracestore.Writer` doc comment (Q3). *Exit:* Cassandra/ClickHouse parity of intent documented; no behavior change yet.
2. **Synchronous bulk primitive.** Provide a synchronous, size-bounded (`max_bytes`) bulk call that returns transport + item-level errors. Prefer implementing on the RFC 0006 `BulkWriter` (§5); if landing earlier, wrap `client.Bulk().Do(ctx)` behind the same internal interface so M6 can swap it. *Exit:* byte-cap + item-error propagation proven by unit test.
3. **`write_mode` config.** Add `write_mode: async|sync` (default `async`), reuse `max_bytes` as chunk cap. *Exit:* config parse/validate + defaults tests.
4. **Wire sync path.** `TraceWriter.WriteTraces` → `WriteSpans` → synchronous bulk; append service/operation docs into the same request; update the service cache only after success (§4.3). *Exit:* `WriteTraces` returns real errors; integration test asserts a failing ES rejects the batch and the error propagates (and, on the ingester, the Kafka offset is **not** committed).
5. **Ingester validation.** End-to-end test on the Kafka path: kill/reject ES mid-stream, assert no offset advance and full recovery on ES return (no data loss). *Exit:* at-least-once demonstrated.
6. **Docs.** Document `write_mode`, the batch-processor guidance (Q2), and the durability-vs-searchability note (§2.2). *Exit:* configuration guide updated.

---

## 9. References

- [Issue #8476 — v2 Elasticsearch WriteTraces cannot propagate write failures](https://github.com/jaegertracing/jaeger/issues/8476)
- [PR #8281 — synchronous bulk in v2 (closed)](https://github.com/jaegertracing/jaeger/pull/8281)
- [PR #8651 — surface async bulk-write errors (draft)](https://github.com/jaegertracing/jaeger/pull/8651)
- [Issue #2192 — unbounded bulk memory / 413](https://github.com/jaegertracing/jaeger/issues/2192)
- [Issue #7612 — Replace olivere/elastic driver](https://github.com/jaegertracing/jaeger/issues/7612)
- [RFC 0004 — Data Streams](./0004-elasticsearch-data-streams.md)
- [RFC 0006 — Unified Elasticsearch/OpenSearch Client](./0006-unified-elasticsearch-client.md)
- [Elasticsearch translog & durability](https://www.elastic.co/docs/reference/elasticsearch/index-settings/translog)
- [Elasticsearch `refresh` parameter](https://www.elastic.co/docs/reference/elasticsearch/rest-apis/refresh-parameter)
- [OpenSearch Bulk API](https://docs.opensearch.org/latest/api-reference/document-apis/bulk/)
- [ClickHouse asynchronous inserts (`async_insert`, `wait_for_async_insert`)](https://clickhouse.com/docs/optimize/asynchronous-inserts)
- [olivere/elastic BulkProcessor](https://github.com/olivere/elastic/wiki/BulkProcessor)
- [go-elasticsearch `esutil.BulkIndexer`](https://pkg.go.dev/github.com/elastic/go-elasticsearch/v8/esutil)
