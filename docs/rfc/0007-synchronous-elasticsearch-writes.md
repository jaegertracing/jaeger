# RFC 0007: Synchronous Elasticsearch/OpenSearch Writes

- **Status:** Draft
- **Author:** Yuri Shkuro
- **Created:** 2026-07-06
- **Last Updated:** 2026-07-06
- **Issue:** [#8476](https://github.com/jaegertracing/jaeger/issues/8476)
- **Related:** [RFC 0004 Data Streams](./0004-elasticsearch-data-streams.md) · [RFC 0006 Unified ES Client](./0006-unified-elasticsearch-client.md) · [#7612](https://github.com/jaegertracing/jaeger/issues/7612) · [#2192](https://github.com/jaegertracing/jaeger/issues/2192) · PRs [#8281](https://github.com/jaegertracing/jaeger/pull/8281), [#8651](https://github.com/jaegertracing/jaeger/pull/8651)

---

## Abstract

Jaeger's Elasticsearch/OpenSearch (ES/OS) trace writer enqueues spans into an **asynchronous** client-side bulk buffer and returns success to its caller **before** the data is durable in the backend. This silently violates the `tracestore.Writer` contract — `WriteTraces` returns `nil` even when the eventual bulk flush fails — and, on the Kafka ingester path, causes Jaeger to **commit Kafka offsets for data that was never persisted**, i.e. silent trace loss on backend outage or overload ([#8476](https://github.com/jaegertracing/jaeger/issues/8476)).

This gap is **unchanged by [RFC 0006 M6](./0006-unified-elasticsearch-client.md#8-migration-plan)** ([#8944](https://github.com/jaegertracing/jaeger/pull/8944)), which migrated the span write path off `olivere/elastic`'s `BulkProcessor` onto an owned bounded `BulkWriter` over `esutil.BulkIndexer`. M6 fixed the unbounded-memory bug ([#2192](https://github.com/jaegertracing/jaeger/issues/2192)) with a hard byte cap, but the new `BulkWriter.Add` is still fire-and-forget — errors surface only in `OnFailure` callbacks — so `WriteTraces` still returns `nil` at enqueue time. M6 swapped the async buffer; it did not make writes synchronous.

This RFC establishes the facts that shape the fix — a single `_bulk` HTTP request to ES/OS is already **synchronous and durable**; the asynchrony is entirely a client-side artifact — and shows that the ClickHouse-style *server-side* batched-insert model the reporter asked about **does not exist** in ES/OS. It then proposes an **opt-in synchronous write mode**: issue **one synchronous, size-bounded `_bulk` request per batch**, checking item-level results and returning a real error, so the writer contract and Kafka at-least-once are restored. Crucially, batching must then move to a **blocking, coalescing batcher** that holds each caller until its batch is durable — which the OTel `exporterhelper` already provides (`queue.wait_for_result` + `queue.batch`; the blocking and error-fan-out are **confirmed in its source and tests**, §4.2), while the fire-and-forget pipeline `batch` processor must be **removed** because it would re-break the guarantee. At-least-once additionally requires the Kafka receiver's `message_marking.after: true` (its default marks offsets *before* the write). Both modes ship; the operator chooses, and the required end-to-end configuration is documented.

This is a design exploration, not a committed decision. It builds directly on the owned `esclient.BulkWriter` M6 delivered, adding a complementary **synchronous** write primitive over the same transport.

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

`WriteSpan` ultimately calls `s.bulkWriter.Add(esclient.BulkItem{…})` ([`core/writer.go`](../../internal/storage/v2/elasticsearch/tracestore/core/writer.go)). `BulkWriter.Add` returns **nothing** ([`esclient/interfaces.go`](../../internal/storage/elasticsearch/esclient/interfaces.go) — `Add(item BulkItem)`); it encodes the document and hands it to the background `esutil.BulkIndexer` ([`esclient/bulk.go`](../../internal/storage/elasticsearch/esclient/bulk.go)). The buffer flushes later on its own triggers, and per-item failures surface only in the indexer's `OnFailure` callback, which logs and updates the `bulk_index` metrics but **cannot influence the already-returned `WriteTraces`**. (Before M6 this was `olivere`'s `BulkProcessor` with an `After` callback — same fire-and-forget shape, different library.)

Cassandra and ClickHouse v2 writers honor the same contract synchronously (they `errors.Join` per-span failures / return `Append`/`Send` errors). **ES is the outlier.**

### 1.2 Why it matters: Kafka silent data loss

Jaeger v2 is an OpenTelemetry Collector assembly. The ingester pipeline is:

```
kafka receiver ──▶ batch processor ──▶ jaeger_storage_exporter ──▶ WriteTraces
```

(`config-kafka-ingester.yaml`). The storage exporter wraps `WriteTraces` in `exporterhelper` with **no sending queue and retry disabled by default** ([`storageexporter/factory.go`](../../cmd/jaeger/internal/exporters/storageexporter/factory.go), [`config.go`](../../cmd/jaeger/internal/exporters/storageexporter/config.go)), so `ConsumeTraces → pushTraces → WriteTraces` is a straight synchronous call — **the only asynchronous hop is the `esutil.BulkIndexer` buffer inside the writer.**

There are actually **two** independent commit-before-durable gaps, and both must be closed for at-least-once. Verified against the receiver's source (the contrib `kafkareceiver` v0.155.0 is **franz-go**-based, `consumer_franz.go`):

1. **Receiver default marks the offset *before* the write even runs.** With the default `message_marking.after: false`, the consume loop calls `client.MarkCommitRecords(msg)` **before** `handleMessage` (i.e. before `ConsumeTraces`) — the offset is queued for commit regardless of the downstream result. Only `message_marking.after: true` moves the mark after a successful `ConsumeTraces`, and only then does a returned error rewind/pause the partition instead of advancing it (`on_error: false`, the default). **So at-least-once requires `message_marking.after: true` no matter how the writer behaves.**
2. **The writer returns `nil` before the data is durable.** Even with `after: true`, `WriteTraces` returns at *enqueue* time, so the receiver marks a batch that is not yet in ES. The sequence:
   1. Ingester reads a Kafka message (a batch of spans).
   2. `WriteTraces` enqueues them into `esutil.BulkIndexer` and returns `nil`.
   3. Receiver marks the offset (before-write by default, or on the bogus success with `after: true`).
   4. **Later**, the indexer flushes — and if ES is down, overloaded, or rejects the mapping, the batch is lost.

The offset is already gone. This converts any transient backend problem into **permanent, invisible trace loss**, and defeats the entire point of buffering traces in Kafka. The same gap removes backpressure: the pipeline cannot slow down or apply retry because it never learns the write failed. This RFC fixes gap (2); the required `message_marking.after: true` for gap (1) is an operator configuration this RFC documents (§4.5) rather than a code change.

### 1.3 Existing attempts

- **[PR #8281](https://github.com/jaegertracing/jaeger/pull/8281)** (closed) — replaced the async processor in the v2 path with a synchronous `client.Bulk().Do(ctx)` plus `resp.Errors` checking. The right direction, closed pending a design decision.
- **[PR #8651](https://github.com/jaegertracing/jaeger/pull/8651)** (open, draft) — "sticky error": record the last async bulk error behind an `atomic.Pointer` and return it on the *next* `WriteTraces`. A minimal patch that closes the contract gap in the loosest sense but attributes the error to the wrong batch and still commits the failing batch's offset.

This RFC exists to pick the direction those PRs were waiting on.

---

## 2. Background: how ES/OS writes actually work

Three facts, each load-bearing for the design and each easy to get wrong.

### 2.1 A `_bulk` request is synchronous and durable

A single `_bulk` HTTP request is **not** asynchronous on the server. Under the default `index.translog.durability: request`, ES/OS `fsync` and commit the translog on the primary **and every allocated replica** before responding, so a `200` means *"all acknowledged writes have been committed to disk"* ([ES translog settings](https://www.elastic.co/docs/reference/elasticsearch/index-settings/translog)). OpenSearch is identical. The `async` durability mode only changes *when* the fsync happens (every `sync_interval`, default 5s) — it does not make the request itself asynchronous, and is a separate, orthogonal knob.

**Implication:** durability is a property Jaeger already gets for free from a single bulk round-trip. The async behavior Jaeger exhibits is **purely client-side** — an artifact of the client-side bulk buffer (`esutil.BulkIndexer` today), not of ES.

### 2.2 Durable ≠ searchable (and that's fine here)

Search visibility is a *separate* axis governed by **refresh** (`index.refresh_interval`, default `1s`; the `refresh` bulk parameter defaults to `false`). A normal bulk `200` means the docs are **durable**, not necessarily **searchable** yet ([refresh parameter](https://www.elastic.co/docs/reference/elasticsearch/rest-apis/refresh-parameter)).

This distinction does **not** affect this RFC: the writer contract and Kafka at-least-once only require **durability** (no acknowledged-but-lost data). We deliberately do **not** propose `refresh=wait_for`/`true` on writes — forcing refresh would wreck indexing throughput for no correctness benefit. Near-real-time search remains the existing ~1s behavior.

### 2.3 There is no server-side "async insert" in ES/OS

The reporter asked whether ES has a ClickHouse-style server-side batched insert — where the **server** coalesces many independent client requests into one flush and can block the client until that flush completes.

**ClickHouse has this.** `async_insert=1` writes incoming INSERTs into a **server-side** in-memory buffer flushed on thresholds (`async_insert_max_data_size` ≈100 MiB, `async_insert_busy_timeout_ms` ≈200 ms, `async_insert_max_query_number` =450), *"invisible to clients … merging insert traffic from multiple sources."* With `wait_for_async_insert=1` (default) the client's INSERT **blocks until the buffered batch is flushed to disk**, yielding a synchronous durability ack on top of server-side batching ([ClickHouse async inserts](https://clickhouse.com/docs/optimize/asynchronous-inserts)). This is exactly the model the reporter described.

**ES/OS have no equivalent.** There is no server-side mode that buffers across separate client requests with an optional wait-for-flush ack. The only batching primitives are:

- the `_bulk` API — batches many docs, but within *one* client request; no cross-request coalescing;
- client-side buffering helpers (`esutil.BulkIndexer` — what Jaeger now uses — olivere `BulkProcessor`, Logstash/Beats) — batching lives in the *client*;
- `translog.durability: async` — an fsync-timing knob, still per-request, not coalescing.

So the ClickHouse option is off the table for ES/OS. The equivalent has to be built where ES puts it — **at the client / pipeline** — which is precisely what this RFC does: let the pipeline (Kafka + batch processor) form the batch, and make the *client's* per-batch write synchronous. (The one place ES could "block until a batch fills" is the client-side buffer's `FlushInterval` — but that blocks *nothing* and acks *nothing*, which is the bug.)

### 2.4 The current async knobs

Post-M6, the `esutil.BulkIndexer` is configured from the same `bulk_processing` block ([`config.go`](../../internal/storage/elasticsearch/config/config.go)), mapped to the indexer's `FlushBytes` / `FlushInterval` / `Workers` ([`esclient/bulk.go`](../../internal/storage/elasticsearch/esclient/bulk.go)); Jaeger defaults: `max_bytes` 5 MB, `flush_interval` 200 ms, `workers` 1. `FlushBytes` is now a **hard byte ceiling** — the buffer flushes before exceeding it — which is what closed the unbounded-memory / `413 Request Entity Too Large` bug [#2192](https://github.com/jaegertracing/jaeger/issues/2192) in M6. (`max_actions` no longer maps to anything: `esutil` has no doc-count trigger. This is a minor config-surface wrinkle M6 introduced, orthogonal to this RFC.) These knobs shape *client-side* flushing; none of them make a write synchronous or its failure observable to the caller.

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

| Criterion | A. Async status quo (post-M6) | B. Sticky-error (#8651) | C. Per-call buffer drain | **D. Sync batch write (rec.)** |
|---|:--:|:--:|:--:|:--:|
| Contract (real errors) | 🔴 always `nil` | 🟡 delayed, best-effort | 🟢 | 🟢 |
| Correct offset / at-least-once | 🔴 | 🔴¹ | 🟢 | 🟢 |
| Attribution | 🔴 | 🔴¹ | 🟢 | 🟢 |
| Throughput | 🟢 | 🟢 | 🔴² | 🟡³ |
| Backpressure | 🔴 | 🔴 | 🟡 | 🟢 |
| Complexity | 🟢 exists | 🟢 small | 🔴² | 🟡 moderate |

Legend / footnotes:
- ¹ The error is recorded against a later call; the batch that actually failed has **already** had its offset committed. At-least-once is not restored.
- ² A per-call synchronous drain of the shared buffer would serialize every caller onto one flush and destroy coalescing. Worse post-M6: `esutil.BulkIndexer` exposes **no** per-call flush — only `Close()` (a full shutdown-flush). Approximating "drain now" means closing and recreating the indexer per batch, which is absurd. Option C is effectively unavailable on the current library, not merely slow.
- ³ Efficiency now depends on the **pipeline** delivering appropriately sized batches (§4.2). With a well-sized batch it is one efficient bulk per batch; with a firehose of tiny batches it degrades to many small bulks (mitigated by upstream batching, which already exists).

**Option D is recommended.** It is the only option that restores both the contract *and* Kafka at-least-once with correct attribution, and it is the model the reporter proposed. C is rejected (throughput). B is a stop-gap that does not fix the data-loss bug for the failing batch. A is the bug.

---

## 4. Recommended design: synchronous batch write

### 4.1 One synchronous bulk per pipeline batch

Replace the per-span enqueue with a per-batch synchronous write:

1. `WriteTraces` converts the whole `ptrace.Traces` batch to `[]dbmodel.Span` (as today).
2. It assembles **one** `_bulk` body containing every span document **and** any service/operation documents the batch requires (§4.3), split into size-bounded sub-requests (§4.4).
3. It issues the bulk **synchronously** via a new blocking primitive on `esclient` (§5) — one `_bulk` round-trip that returns the parsed response — inspects `resp.Errors` and each item's status, and **returns an error** if the transport failed or any item failed.
4. Only on `nil` does the exporter return success → the Kafka offset commits.

This requires the core writer to expose a batch, error-returning entry point (the `#8476` discussion sketched exactly this):

```go
// core.Writer
WriteSpans(ctx context.Context, spans []dbmodel.Span) error   // replaces fire-and-forget WriteSpan
```

`TraceWriter.WriteTraces` calls it once per batch and returns its error verbatim. The `SpanWriter`'s existing responsibilities (tag elevation, index/rotation target selection, `@timestamp` for data streams, service dedup cache) are unchanged — only the *sink* changes from "enqueue into shared buffer" to "append into this batch's bulk request."

### 4.2 Batching without breaking the guarantee

Synchronous writes move the batching problem, they do not remove it. Batch **size** now determines throughput, so batches must still be formed somewhere — but that "somewhere" **must block the origin caller until its spans are durably written**, or the whole guarantee is lost again. This is the central design constraint, and it has one sharp consequence:

**The OTel `batch` processor must be removed for sync mode.** The pipeline `batch` processor is **fire-and-forget**: `ConsumeTraces` buffers the spans and returns `nil` immediately, and the batch is flushed to the exporter later on a separate goroutine. Placed in front of a synchronous writer, it **re-introduces the exact bug this RFC fixes** — the Kafka offset commits when the processor buffers, not when ES persists. So sync mode with the `batch` processor is strictly worse than useless. It has to go.

That leaves the question the reporter raised: if the pipeline no longer batches, and upstream messages are small (SDKs or a default-config collector emitting small `ptrace.Traces`), then **one synchronous bulk per message** is too small and throughput suffers. Sizing bulks well *and* keeping writes synchronous requires a **blocking, coalescing batcher** — one that merges spans from many concurrent `ConsumeTraces` calls into one large bulk and unblocks each caller with that bulk's durable result. This is precisely the ClickHouse `wait_for_async_insert=1` model (§2.3), realized in the collector rather than the server.

**This mechanism already exists in the OTel `exporterhelper` the storage exporter is built on, and its behavior is confirmed by reading the source (v0.155.0), not just the config.** `exporterhelper`'s `QueueBatchConfig` (which the storage exporter already wires via `WithQueue`, [`storageexporter/factory.go`](../../cmd/jaeger/internal/exporters/storageexporter/factory.go)) exposes `wait_for_result` and a nested `batch` block. Tracing the code:

- **Blocking (`wait_for_result: true`).** `memoryQueue.Offer` creates a `blockingDone{ch chan error}`, enqueues, then blocks on `<-done.ch`; the consumer signals it with `bd.ch <- err` **after export**. So the calling goroutine returns the *export* result, not an enqueue ack. (`internal/queue/memory_queue.go`.)
- **Coalescing + result fan-out.** The `partitionBatcher` accumulates each caller's `done` into a `multiDone []queue.Done`; on flush it runs `done.OnDone(consumeFunc(ctx, req))`, and `multiDone.OnDone` loops calling **every** merged caller's `done` with the **same single error** (`internal/queuebatch/partition_batcher.go`). Framework test `TestPartitionBatcher_MergeError` asserts exactly this fan-out; `TestQueueBatch_BatchBlocking` asserts concurrent `Send`s block until the batch flushes. Both `wait_for_result` and `batch` may be set together (the only rejected combination is `wait_for_result` with a **persistent** disk queue — irrelevant here, since sync mode wants the durable ack, not a spooling queue).

So the required semantics are not merely plausible — they are what the code does: N partition-consumer `ConsumeTraces` calls merge into one bulk, each blocks until that bulk is durably written, and **all** of them receive its error. This resolves the concern flagged in the prior draft. One real consequence remains (§4.6): the merge (`MoveAndAppendTo`) fuses all callers' spans, so the result is strictly **all-or-nothing** — there is no per-caller partial result.

Three ways to form the batch:

1. **Exporter-level blocking batcher** (recommended, available today) — `queue.wait_for_result: true` + `queue.batch` on the storage exporter, pipeline `batch` processor removed. Config-only, no new code. Works for every source (Kafka *and* direct OTLP) because it coalesces at the write boundary regardless of upstream message size, including across concurrent Kafka partitions.
2. **Receiver-level batching** (the reporter's suggestion — simpler offsets, but not available today). Have the Kafka receiver combine the N records it already fetches per partition (`kgo.FetchTopicPartition.Records`) into **one** `ConsumeTraces` call, then mark all N offsets together. This is appealing: one caller ⇒ one bulk ⇒ one all-or-nothing result ⇒ trivial, monotonic per-partition offset handling — no cross-partition coalescing, no `multiDone` fan-out, no `wait_for_result` machinery. **However, the current contrib receiver does not do this**: verified in `consumer_franz.go`, the loop is `for _, msg := range msgs { handleMessage(pc, msg) }` — strictly one record per `ConsumeTraces`, with no message-count/batch config knob. Realizing it means an upstream contrib feature (accumulate per-partition records before the consumer call) or a thin Jaeger-owned receiver wrapper. Worth pursuing upstream, but it is code, not config.
3. **Custom synchronous batch processor** (fallback) — a Jaeger processor that buffers concurrent `ConsumeTraces` calls, flushes one bulk on size/time, and blocks each caller until the flush completes. Same semantics as (1) but owned by us; only worth building if (1) proves unfit.

Comparison (🟢 good · 🟡 partial/caveated · 🔴 poor):

| Criterion | 1. Exporter batcher | 2. Receiver batching | 3. Custom processor |
|---|:--:|:--:|:--:|
| Available today (no new code) | 🟢 config-only | 🔴 needs upstream/custom receiver | 🔴 new component |
| Coalesces across partitions | 🟢 | 🔴 per-partition only¹ | 🟢 |
| Offset/commit simplicity | 🟡 many blocked callers | 🟢 one caller, mark N together | 🟡 |
| Per-caller failure attribution | 🔴 all-or-nothing² | 🟡 whole-partition batch² | 🟡 could preserve, if built for it |
| Works for direct-OTLP (non-Kafka) | 🟢 | 🔴 Kafka-only | 🟢 |

- ¹ Per-partition batching is often *enough*: more partitions ⇒ more concurrent bulks, which is good for ES throughput. Cross-partition coalescing mainly helps low-partition-count topics.
- ² See §4.6 — no mechanism maps ES per-item failures back to individual callers/messages in (1) or (2).

**Recommendation:** ship with (1) — it exists, it is config-only, and it covers both Kafka and direct-OTLP. Pursue (2) upstream in parallel as the cleaner long-term shape for the Kafka ingester (simplest offsets). Keep (3) as a fallback only.

The redundant client-side buffer (`esutil.BulkIndexer`) is removed in sync mode. Batching happens **once**, at the blocking batcher, where it is observable and tunable — not hidden in a fire-and-forget client buffer that discards the durability signal.

### 4.3 Service/operation documents

`WriteSpan` today also writes a `service:operation` dedup document per new (service, operation) pair, gated by an in-memory TTL cache ([`core/service_operation.go`](../../internal/storage/v2/elasticsearch/tracestore/core/service_operation.go)). These documents already carry a **deterministic `_id`** — the FNV-64a `hashCode(service)` of `{serviceName, operationName}` — so repeated writes are idempotent upserts on the ES side; retries never duplicate them.

Two adjustments for sync mode:

- **Same bulk.** The service document is appended to the **same** bulk request as the spans of the batch, so it shares their durability and error signal.
- **Cache after durable write.** Today the writer calls `writeCache(cacheKey, …)` immediately after `Add()` — i.e. it marks the service "written" at *enqueue* time. In sync mode the cache must be updated only **after** the bulk succeeds; otherwise a failed-then-retried batch would find the key cached, skip the service document on retry, and — although the ES-side hash `_id` makes the document itself recoverable if *any* later span re-emits it — risk leaving a gap until then. Moving the cache write past the durable ack closes that window. (Same "cache after durable write" discipline as [RFC 0004 §3.9](./0004-elasticsearch-data-streams.md).)

### 4.4 Size bounding, retries, and duplicates

- **Size bound.** Even a well-formed batch could exceed `http.max_content_length` (default 100 MB) and 413. The primary control is the batcher's `max_size` (§4.2/§4.5), but the sync writer keeps a safety-net cap at `max_bytes`: it splits an oversized batch into sub-requests, issues them in sequence, and joins their errors. This is the same byte discipline M6's async `FlushBytes` already enforces ([#2192](https://github.com/jaegertracing/jaeger/issues/2192) is fixed for the async path) — the sync path must carry it too rather than inherit it, since it does not go through `esutil`'s buffer.
- **Retry.** On a returned error the batch is retried (Kafka re-delivery, or `exporterhelper` retry). Item-level `429/503` (backpressure) therefore retry naturally. We do **not** silently swallow partial failures.
- **Duplicates / idempotency.** For **spans**, Jaeger sets no `_id` (server-generated), so a retry can create duplicate span docs. This is **already** the case today on any retry and is tolerated (append-only, at-least-once; see [RFC 0004 §3.4](./0004-elasticsearch-data-streams.md)); synchronous retry makes it *more visible*, not new. Note this is unlike the **service** documents, which already use a deterministic hash `_id` and so upsert idempotently (§4.3) — the span path could adopt the same pattern (a deterministic `_id` of `traceID+spanID+startTime` with `op_type=create`) to get at-least-once span dedup for free. That is cross-referenced as a future improvement in RFC 0004 and kept out of scope here to avoid coupling two decisions.

### 4.5 Configuration

Two independent settings, on two components. First, the storage backend's write mode:

```yaml
elasticsearch:
  write_mode: async   # async (default) | sync
  bulk_processing:     # shapes async esutil.BulkIndexer flushing; in sync mode only `max_bytes` (safety-net chunk cap)
    max_bytes: 5000000
    flush_interval: 200ms
    workers: 1
```

- `async` (default): today's `esutil.BulkIndexer` behavior (post-M6), unchanged — fully backward compatible.
- `sync`: §4.1. `max_bytes` is reused as the safety-net chunk ceiling (§4.4); `flush_interval` and `workers` are inert (each write is one blocking round-trip).

Second — and this is the part operators must get right — the **pipeline** must batch with a *blocking* batcher and must **not** use the fire-and-forget `batch` processor (§4.2). The recommended shape for the Kafka ingester:

```yaml
service:
  pipelines:
    traces:
      receivers: [kafka]
      processors: []                     # NO batch processor in sync mode (it is fire-and-forget)
      exporters: [jaeger_storage_exporter]

receivers:
  kafka:
    message_marking:
      after: true                        # REQUIRED: mark offset only after ConsumeTraces succeeds
      on_error: false                    # (default) failed write → rewind/pause partition, do not advance

exporters:
  jaeger_storage_exporter:
    trace_storage: some_storage
    queue:
      wait_for_result: true              # block each caller until its batch is durable
      # note: wait_for_result is incompatible with a persistent (storage:) queue
      batch:
        sizer: bytes
        min_size: 5000000                # coalesce small messages up to ~5 MB
        max_size: 10000000               # hard cap per bulk (413 protection)
        flush_timeout: 200ms             # bound added latency for low-traffic streams
```

Three things must line up, and all are the **operator's** choice — Jaeger ships both modes and does not pick for them:

1. `elasticsearch.write_mode: sync` (the durable writer),
2. the Kafka receiver's `message_marking.after: true` (§1.2 — otherwise the offset commits before the write regardless), and
3. a blocking pipeline batcher (`queue.wait_for_result` + `queue.batch`) with the fire-and-forget `batch` processor removed (§4.2).

Any one of the three missing silently degrades the guarantee: `write_mode: sync` with a `batch` processor still present, or with `message_marking.after: false`, loses data exactly as before. This coupling **must be documented prominently** (§8). A startup warning when `write_mode: sync` coexists with a pipeline `batch` processor is worth considering, though the exporter cannot see the full pipeline graph (§7 Q2).

### 4.6 Kafka concurrency, offset commits, and fine-grained acking

**Concurrency and offsets are handled by the receiver + franz-go — Jaeger does not hand-roll them.** This is worth stating because Jaeger v1's Kafka consumer *did* track offsets manually and it was error-prone. In the contrib franz-go receiver (`consumer_franz.go`, verified):

- Each poll dispatches **one goroutine per partition** (`fetch.EachPartition(... go func(pc, msgs))`); partitions are processed **concurrently**, and within a partition records are processed **serially** (`for _, msg := range msgs`).
- Offsets are marked/committed by the library in **monotonic per-partition order** (`MarkCommitRecords` + `CommitMarkedOffsets`/autocommit); the integrator never computes offsets. On a failed write with `after: true`, the partition is **rewound to the failed record** (`SetOffsets`) or **paused**, so nothing past the failure commits.

So "consume several chunks in parallel and commit their offsets independently" is exactly what happens: parallelism = number of partitions, each committing its own contiguous, monotonic range. Those concurrent per-partition `ConsumeTraces` calls are precisely the callers the exporter batcher (§4.2 option 1) coalesces and then unblocks together.

**Fine-grained acking — is there room to use ES per-item bulk results?** The reporter asked whether, since the ES `_bulk` response reports per-document status, the ack could be finer than all-or-nothing (commit the messages whose docs all succeeded, retry only the ones that failed). The answer, from the code, is **not with the built-in batcher, and not cheaply**:

- The exporter batcher's `MergeSplit` fuses all callers' spans via `ResourceSpans().MoveAndAppendTo(...)` **before** the export. By the time the bulk runs, the mapping *"this `_bulk` item ↔ this Kafka message/offset"* is gone. `consumeFunc` returns a single `error`; `multiDone` gives every caller that same error. ES per-item results have nowhere to be routed back to.
- To make acking fine-grained you would need to **preserve per-message span ranges through to the bulk response** and translate each rejected item's ordinal back to its source message — i.e. *not* use the merging batcher, and instead a custom writer/batcher that keeps the message boundaries and maps `resp.items[i].status` → offset. That is real complexity for limited payoff: ES partial-bulk failures cluster into two regimes — **whole-target unavailable** (every item fails; all-or-nothing retry is already correct) and **poison documents** (a few items rejected for mapping/validation reasons that will fail identically on every retry, so per-message retry just loops). Neither regime is meaningfully helped by finer acking; the poison case argues instead for a dead-letter path, which is orthogonal.
- Receiver-level batching (§4.2 option 2) is coarser still — one partition's fetch is one unit; on any failure the whole partition batch rewinds. But its offset story is trivial, which is the point.

**Recommendation:** keep acking **all-or-nothing per batch**. It is correct for at-least-once (worst case: a retry re-writes some already-durable spans → duplicate span docs, already tolerated, §4.4). Treat fine-grained per-item acking and dead-lettering of poison documents as a separate future item, not part of this RFC.

---

## 5. Building on RFC 0006 and RFC 0004

This work is small and lands cleanly on top of the client foundation [RFC 0006](./0006-unified-elasticsearch-client.md) is building:

- **RFC 0006 M6 has merged** ([#8944](https://github.com/jaegertracing/jaeger/pull/8944)): the owned `esclient.BulkWriter` and the bounded bulk indexer over `esutil.BulkIndexer` exist and carry all span/service writes; `olivere` is gone from the write path (it remains only for the reader/control-plane, being migrated by later 0006 milestones). M6 delivered the **async** half; this RFC adds the **synchronous** half over the same `esclient` transport. Because `esutil.BulkIndexer` is async-only (no blocking round-trip), the sync path is a **new, distinct primitive** — a blocking bulk on `esclient` (e.g. `Bulk(ctx, items) (BulkResult, error)`), reusing 0006's owned request/response types and the shared transport pool. It is **not** a method on the existing async `BulkWriter`; the two are peers selected by `write_mode`. Concretely: `SpanWriter` holds either the async `BulkWriter` (today) or a synchronous bulk client, chosen at construction from config.
- **RFC 0004 (Data Streams)** already changes the write op-type (`index`→`create`) and adds `@timestamp`. The sync writer inherits both transparently — it writes whatever documents `SpanWriter` produces (which already emits `esclient.BulkItem`s with the right `OpType`). The service-doc cache ordering (§4.3) reuses the same "cache after durable write" discipline data streams' trace-timestamp index needs.

There is no conflict; there is shared surface. Treat "synchronous write mode" as a peer of M6's async `BulkWriter` on the same `esclient`, with the contract/Kafka fix (this RFC) as the motivating requirement for the synchronous primitive.

---

## 6. Migration & backward compatibility

- **Default unchanged.** `write_mode: async` remains the default; existing deployments see no behavior change.
- **Opt-in.** Operators set `write_mode: sync` **together with** a blocking pipeline batcher and removal of the fire-and-forget `batch` processor (§4.2/§4.5). Correctness improves (no acked-but-lost data); throughput becomes a function of the batcher's size settings.
- **No schema/on-disk change.** Documents, indices, mappings, and read paths are identical. This is purely a write-*mechanism* change.
- **Rollback.** Flip back to `write_mode: async`. No data migration.

---

## 7. Open questions

**Q1 — Exporter batcher (now) vs. receiver-level batching (later)?**
The blocking-and-fan-out semantics of `queue.wait_for_result` + `queue.batch` are **confirmed in the `exporterhelper` v0.155.0 source and its tests** (§4.2), so option 1 is safe to ship as config-only. The genuinely open item is whether to *also* invest in receiver-level per-partition batching (§4.2 option 2) — simpler offsets, but needs an upstream contrib change or a Jaeger receiver wrapper. Recommendation: ship option 1; open an upstream issue for option 2 and adopt it if accepted. Pin the `exporterhelper` version, since the batcher internals are `internal/` and could change.

**Q2 — Startup validation of the sync + batch-processor misconfiguration.**
`write_mode: sync` with a fire-and-forget `batch` processor still in the pipeline silently defeats the guarantee (§4.5). Can the exporter detect this at startup and warn/fail? The exporter does not see the full pipeline graph, so detection may not be feasible from inside the component; the fallback is prominent documentation. Recommendation: investigate a collector-level check; ship docs regardless.

**Q3 — Interface doc for `WriteTraces` async allowance.**
Regardless of mode, the `tracestore.Writer` doc comment should state explicitly whether an implementation may be asynchronous. Today the comment implies synchronous semantics that only Cassandra/ClickHouse honor. Recommendation: document that `WriteTraces` **must** return write errors synchronously; `async` mode is a deliberate, documented deviation for throughput at the cost of the guarantee.

**Q4 — Deterministic `_id` for span dedup?**
Service documents already use a deterministic hash `_id` (§4.3); spans do not. Adopting the same pattern for spans (`traceID+spanID+startTime`, `op_type=create`) would give at-least-once span dedup and make synchronous retries clean. Recommendation: keep out of scope here; track with RFC 0004's identical open question to avoid coupling two decisions. Not blocking for this RFC.

---

## 8. Implementation plan

Each step is independently shippable and guarded by unit + ES/OS integration tests.

1. **Writer contract + core API.** Add `core.Writer.WriteSpans(ctx, []span) error`; keep `WriteSpan` for the async path. Update the `tracestore.Writer` doc comment (Q3). *Exit:* Cassandra/ClickHouse parity of intent documented; no behavior change yet.
2. **Synchronous bulk primitive.** Add a new blocking bulk method on `esclient` (peer of M6's async `BulkWriter`, over the same transport — §5): one `_bulk` round-trip, size-bounded by `max_bytes`, returning transport + item-level errors from 0006's owned response types. *Exit:* byte-cap + item-error propagation proven by unit test.
3. **`write_mode` config.** Add `write_mode: async|sync` (default `async`), reuse `max_bytes` as chunk cap. *Exit:* config parse/validate + defaults tests.
4. **Wire sync path.** `TraceWriter.WriteTraces` → `WriteSpans` → synchronous bulk; append service/operation docs into the same request; update the service cache only after success (§4.3). *Exit:* `WriteTraces` returns real errors; integration test asserts a failing ES rejects the batch and the error propagates (and, on the ingester, the Kafka offset is **not** committed).
5. **Blocking batcher (config, no new code).** Provide/validate the ingester pipeline shape: `queue.wait_for_result: true` + `queue.batch` on the storage exporter, the pipeline `batch` processor removed, and the Kafka receiver's `message_marking.after: true`. The `exporterhelper` fan-out/blocking is already proven by its own tests (§4.2), so this step is an example config + a Jaeger-side integration test, not a new component. *Exit:* an ingester config that nacks every message in a failed batch and advances no offset.
6. **Ingester validation.** End-to-end test on the Kafka path with the sync writer + blocking batcher + `message_marking.after: true`: kill/reject ES mid-stream, assert no offset advance and full recovery on ES return (no data loss). *Exit:* at-least-once demonstrated end-to-end.
7. **Docs.** Document `write_mode` **and** the co-required settings (`message_marking.after: true`, blocking batcher, no `batch` processor — §4.5), upstream-sizing guidance, the durability-vs-searchability note (§2.2), and the all-or-nothing acking rationale (§4.6). *Exit:* configuration guide updated.
8. **(Optional, upstream) Receiver-level batching.** Open a contrib issue/PR to let the Kafka receiver coalesce per-partition records into one `ConsumeTraces` (§4.2 option 2). Independent of 1–7; adopt if accepted.

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
- [OpenTelemetry `exporterhelper` — queue/batch settings (`wait_for_result`, `batch`)](https://github.com/open-telemetry/opentelemetry-collector/blob/main/exporter/exporterhelper/README.md) — blocking + fan-out verified in `internal/queue/memory_queue.go` and `internal/queuebatch/partition_batcher.go` (v0.155.0)
- [OpenTelemetry contrib `kafkareceiver` — `message_marking`, franz-go consumer](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/kafkareceiver) — mark-before/after and per-partition concurrency verified in `consumer_franz.go` (v0.155.0)
- [olivere/elastic BulkProcessor](https://github.com/olivere/elastic/wiki/BulkProcessor)
- [go-elasticsearch `esutil.BulkIndexer`](https://pkg.go.dev/github.com/elastic/go-elasticsearch/v8/esutil)
