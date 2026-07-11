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

### 1.3 How we got here: ES writes were synchronous first

This is not a new capability so much as a return to the original design, made possible again by the v2 pipeline. The ES writer **started synchronous**, exactly like Cassandra:

- **v1 storage interface forced one span per write.** The `spanstore.Writer` interface was `WriteSpan(span *model.Span) error` — a single span, no batch method (introduced in [PR #47](https://github.com/jaegertracing/jaeger/pull/47), `db6587b9`, 2017-02-15; today `internal/storage/v1/api/spanstore/interface.go` with a `ctx`). Any batching therefore had to be hidden *below* the interface.
- **The first ES writer was synchronous, one document per HTTP request.** [PR #200](https://github.com/jaegertracing/jaeger/pull/200) (`f3717836`, 2017-06-19) added `plugin/storage/es/spanstore/writer.go`, whose `WriteSpan` issued blocking `.Do(s.ctx)` calls per document and **returned the write error synchronously** — the same shape Cassandra still has.
- **It was replaced wholesale by the async bulk processor for throughput.** [PR #656 "Use elasticsearch bulk API"](https://github.com/jaegertracing/jaeger/pull/656) (`e52ecffb`, 2018-01-23, Pavol Loffay) swapped the per-span `.Do(s.ctx)` for `.Add()` into a shared background `elastic.BulkProcessor`, and introduced the `.bulk.*` knobs (later the `BulkProcessing` struct, [PR #6090](https://github.com/jaegertracing/jaeger/pull/6090)). The exact diff:
  ```diff
  - _, err := s.client.Index().Index(indexName).Type(spanType).BodyJson(&elasticSpan).Do(s.ctx)
  + s.client.Index().Index(indexName).Type(spanType).BodyJson(&elasticSpan).Add()
  ```
  `writeSpan`/`writeService` lost their error returns in that commit — because with a one-span interface, issuing an HTTP round-trip per span is far too slow at collector scale, and the only way to batch under `WriteSpan(one span)` was a buffer with the outcome reported asynchronously. **The async model was the correct answer to the v1 constraint.**

The v2 `tracestore.Writer` interface removes that constraint — `WriteTraces(ctx, ptrace.Traces)` already hands the writer a whole **batch**. Synchronous batched writes are now expressible without one-request-per-span; the historical reason for going async no longer binds. (M6, [PR #8944](https://github.com/jaegertracing/jaeger/pull/8944), later swapped the olivere processor for an owned `esutil.BulkIndexer`, but preserved the async, fire-and-forget shape — §2.4.)

### 1.4 Existing attempts

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

Goal: **make it *possible*** for `WriteTraces` to return a truthful error for the batch it was given, so that (on Kafka) the offset commits only after that batch is durable — without collapsing throughput. Note "possible", not "always on": async writing is a legitimate mode for the unbatched direct-ingest path (§3.1), so this is about *adding* a synchronous option and choosing sensible defaults, not deleting the async one.

The options:

- **A. Async status quo (post-M6).** Leave the write path as it is: `WriteSpan` enqueues into the async `esutil.BulkIndexer` and `WriteTraces` returns `nil` immediately; failures land in `OnFailure` callbacks that only log and count. This is today's behavior. It is the *bug* for the Kafka-buffered topology (silent loss, §1.2), but — importantly — it is also the *right default* for direct, unbatched ingest (§3.1), which is why it is a real option and not merely a strawman baseline.
- **B. Sticky-error ([#8651](https://github.com/jaegertracing/jaeger/pull/8651)).** Keep the async indexer, but record the most recent bulk error behind an `atomic.Pointer` and return it from the *next* `WriteTraces`. A minimal patch that surfaces *some* error eventually, without changing the async model.
- **C. Per-call buffer drain.** Keep the async indexer but force a synchronous flush of the shared buffer at the end of every `WriteTraces`, then inspect results. Spec-conformant in principle, but it flushes the whole shared buffer per call.
- **D. Synchronous batch write (recommended).** Replace the per-span enqueue with one synchronous, size-bounded `_bulk` per batch: assemble the batch's documents, issue a blocking round-trip, check item-level results, and return a real error (§4). Batching moves to a blocking pipeline batcher (§4.2). This is the model the reporter proposed.

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

The matrix scores against the *Kafka at-least-once* goal. Read that way: **B** is a stop-gap that never withholds the failing batch's offset; **C** is spec-conformant but destroys batching and is anyway unimplementable on `esutil` (footnote 2); **D** is the only option that restores the contract *and* at-least-once with correct attribution, and is the model the reporter proposed. **A** scores red across the correctness rows — but only because those rows encode a goal that does not apply to every topology (§3.1).

So the recommendation is **not simply "pick D"** — it is:

1. **Implement D.** Build the synchronous write primitive (§4, §5); without it the at-least-once topology is impossible.
2. **Recover A's throughput with a batcher, not with async.** A's one virtue is coalescing many spans into few `_bulk` requests. That virtue is *not* exclusive to async: **D + a blocking batcher** (§4.2) — or even D behind the plain `batch` processor, if one is willing to trade the durability guarantee for raw batching — reproduces A's request shape while keeping (or knowingly dropping) the error signal. Choosing D therefore does not cost throughput wherever batching exists.
3. **Keep A as the default for now; phase it toward D via a feature gate.** Async stays the shipped default so no existing deployment changes behavior, gated by a feature gate (e.g. `es.write.synchronous`) that flips the default over the standard Jaeger alpha→beta→stable lifecycle. This gives operators time to add the required pipeline shape (§4.5) before sync becomes default.

### 3.1 When async is the *right* choice, not just legacy

Async writing is not merely the old behavior to be tolerated until removed — it is the correct mode for one real topology: **direct, unbatched ingest**. When spans arrive over TCP straight into the collector (OTLP/Jaeger receivers → storage, no Kafka), there is no natural batch boundary and no offset to protect. Forcing a synchronous `_bulk` per inbound request would mean either tiny, inefficient bulks or a blocking batcher that adds `flush_timeout` latency to every request for a guarantee (at-least-once against a durable log) that this topology does not even have — the client, not a Kafka offset, owns retry. There, fire-and-forget client-side batching is exactly right.

Synchronous writes earn their cost specifically where there **is** a durable buffer to be honest with — the **Kafka ingester** — and where losing an ack means losing data that Kafka still believes was delivered. This is why the two modes coexist: `async` for unbatched direct ingest, `sync` for Kafka-buffered ingest. The feature gate governs the *default*; whether async is ever fully removed (vs. retained as a non-default option for the direct-ingest path) is left open (§7).

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
- **Duplicates / idempotency — now in scope, via a deterministic span `_id` (§4.7).** Today Jaeger sets no `_id` for spans (server-generated), so *any* retry — Kafka re-delivery or `exporterhelper` retry — creates duplicate span docs. Under async fire-and-forget this was a rare, tolerated edge case; under **synchronous at-least-once** writing, retries are the normal error-recovery path, so duplication moves from an edge case to a first-order concern (at high ingest a retry storm multiplies stored volume). The fix is the pattern the **service** documents already use (§4.3) and that **Cassandra has always used** — its `traces` primary key is `(trace_id, span_id, span_hash)`, where `span_hash` is a *content* hash, so re-inserting an identical span is an idempotent upsert while genuinely-distinct spans that share `(trace_id, span_id)` stay separate. Give each ES/OS span a **deterministic content-hash `_id`** and a re-sent identical span upserts instead of duplicating. This makes whole-batch retry idempotent (which is why §4.6 needs no per-chunk retry bookkeeping) and is therefore **promoted from a future RFC 0004 cross-reference to an in-scope milestone here** — at-least-once is not production-safe without it. Two design notes carried into §4.7: (a) `traceID+spanID+startTime` (RFC 0004's sketch) is **insufficient** — the shared-span model lets a client and server span share `(traceID, spanID)`, so a *content* hash is required, as Cassandra does; (b) the `_id`↔`op_type` interaction matters — `op_type: index` upserts cleanly, but data streams force `op_type: create`, under which a retried `_id` returns 409 that the writer must treat as **idempotent success**, not a failure.

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

The **default** value of `write_mode` (when unset) is governed by a feature gate — e.g. `es.write.synchronous` — following Jaeger's standard lifecycle: **alpha** (default `async`, opt into `sync`) → **beta** (default `sync`, opt out to `async`) → **stable**. An explicit `write_mode` in config always wins over the gate, so operators can pin either mode regardless of lifecycle stage (§3, point 3).

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
        min_size: 3000000                # coalesce small messages up to ~3 MB
        max_size: 5000000                # match bulk_processing.max_bytes so the sync writer's safety-net split (§4.4) rarely triggers
        flush_timeout: 200ms             # bound added latency for low-traffic streams
```

Three things must line up, and all are the **operator's** choice — Jaeger ships both modes and does not pick for them:

1. `elasticsearch.write_mode: sync` (the durable writer),
2. the Kafka receiver's `message_marking.after: true` (§1.2 — otherwise the offset commits before the write regardless), and
3. a blocking pipeline batcher (`queue.wait_for_result` + `queue.batch`) with the fire-and-forget `batch` processor removed (§4.2).

Any one of the three missing silently degrades the guarantee: `write_mode: sync` with a `batch` processor still present, or with `message_marking.after: false`, loses data exactly as before. This coupling **must be documented prominently** (§8). A startup warning when `write_mode: sync` coexists with a pipeline `batch` processor is worth considering, though the exporter cannot see the full pipeline graph (§7 Q2).

A related but separate misconfiguration — `queue.batch.max_size` exceeding `bulk_processing.max_bytes` (§4.4) — does not have that visibility problem: both values belong to the exporter itself. `jaeger_storage_exporter`'s `start()` already resolves `trace_storage` to its ES factory via `jaegerstorage.GetTraceStoreFactory`, and the exporter's own `Config` already carries `queue.batch.max_size`, so it can read the resolved factory's `bulk_processing.max_bytes` and compare the two before serving traffic. Because this is a deterministic comparison of two configured integers, not a runtime heuristic that needs operational experience to trust, it should **hard-fail startup** when `write_mode: sync` and `queue.batch.max_size > bulk_processing.max_bytes`, rather than only warn.

### 4.6 Kafka concurrency, offset commits, and fine-grained acking

**Concurrency and offsets are handled by the receiver + franz-go — Jaeger does not hand-roll them.** This is worth stating because Jaeger v1's Kafka consumer *did* track offsets manually and it was error-prone. In the contrib franz-go receiver (`consumer_franz.go`, verified):

- Each poll dispatches **one goroutine per partition** (`fetch.EachPartition(... go func(pc, msgs))`); partitions are processed **concurrently**, and within a partition records are processed **serially** (`for _, msg := range msgs`).
- Offsets are marked/committed by the library in **monotonic per-partition order** (`MarkCommitRecords` + `CommitMarkedOffsets`/autocommit); the integrator never computes offsets. On a failed write with `after: true`, the partition is **rewound to the failed record** (`SetOffsets`) or **paused**, so nothing past the failure commits.

So "consume several chunks in parallel and commit their offsets independently" is exactly what happens: parallelism = number of partitions, each committing its own contiguous, monotonic range. Those concurrent per-partition `ConsumeTraces` calls are precisely the callers the exporter batcher (§4.2 option 1) coalesces and then unblocks together.

**Fine-grained acking — is there room to use ES per-item bulk results?** The reporter asked whether, since the ES `_bulk` response reports per-document status, the ack could be finer than all-or-nothing (commit the messages whose docs all succeeded, retry only the ones that failed). The answer, from the code, is **not with the built-in batcher, and not cheaply**:

- The exporter batcher's `MergeSplit` fuses all callers' spans via `ResourceSpans().MoveAndAppendTo(...)` **before** the export. By the time the bulk runs, the mapping *"this `_bulk` item ↔ this Kafka message/offset"* is gone. `consumeFunc` returns a single `error`; `multiDone` gives every caller that same error. ES per-item results have nowhere to be routed back to.
- To make acking fine-grained you would need to **preserve per-message span ranges through to the bulk response** and translate each rejected item's ordinal back to its source message — i.e. *not* use the merging batcher, and instead a custom writer/batcher that keeps the message boundaries and maps `resp.items[i].status` → offset. That is real complexity for limited payoff: ES partial-bulk failures cluster into two regimes — **whole-target unavailable** (every item fails; all-or-nothing retry is already correct) and **poison documents** (a few items rejected for mapping/validation reasons that will fail identically on every retry, so per-message retry just loops). Neither regime is meaningfully helped by finer acking; the poison case argues instead for a dead-letter path, which is orthogonal.
- Receiver-level batching (§4.2 option 2) is coarser still — one partition's fetch is one unit; on any failure the whole partition batch rewinds. But its offset story is trivial, which is the point.

**Recommendation:** keep offset attribution **all-or-nothing per batch** — but this is *not* the same as ignoring per-item results. It is correct for at-least-once because a retry of already-durable spans is a no-op once they carry a deterministic `_id` (§4.7), not a duplicate. What all-or-nothing does **not** by itself solve is the **poison document**: a doc ES/OS rejects deterministically (mapping conflict, malformed field). Failing the batch forever on a poison doc means the offset never advances and the partition is re-delivered in an infinite loop — **head-of-line blocking**. At production ingest (e.g. 100 MB/s) there is no "operator intervenes": a stalled partition is an outage, and back-pressure propagates until the pipeline falls over. So poison handling is **not** a deferrable "future item" — a conformant synchronous writer must dispose of poison documents **autonomously** so the partition always makes progress. That mechanism is specified in §4.8; the per-item `_bulk` results this writer already parses are used there to classify terminal vs. transient failures (not to attribute offsets).

### 4.7 Idempotent span writes: a deterministic content-hash `_id`

Synchronous at-least-once makes retries a normal, frequent event, so the write must be **idempotent**: a re-sent span must not create a second document. Service documents already achieve this with a deterministic hash `_id` (§4.3); Cassandra has always achieved it by putting a content hash (`span_hash`) in the span primary key. Spans in ES/OS get the same treatment:

- **`_id` = a content hash of the span.** Compute a stable hash over the span's canonical bytes (as Cassandra's `dbmodel.Span.Hash` does — gob-encode the span with the hash field zeroed; the ES writer would hash its own `dbmodel.Span`). A re-sent identical span produces the same `_id` and does not duplicate.
- **Why not `traceID+spanID+startTime`.** RFC 0004 sketched that key, but it is unsafe: in the shared-span (Zipkin-style) model a client and server span legitimately share `(traceID, spanID)` and can share `startTime`; keying on that would collapse two distinct spans into one. A content hash distinguishes them, exactly as Cassandra's `span_hash` does — which is why we match Cassandra rather than the sketch.
- **`op_type` interaction (load-bearing).** With `op_type: index` (legacy indices) a duplicate `_id` **overwrites** — a clean idempotent upsert, no special handling. With `op_type: create` (**required by data streams**, RFC 0004) a duplicate `_id` is rejected **409 version_conflict**; the synchronous writer must recognize a 409 on its *own* deterministic `_id` as **idempotent success** (the span is already durable) and **not** surface it as a batch error. Without that, every legitimate retry under data streams would look like a failure and stall the offset.
- **Cost.** A client-supplied `_id` disables ES's auto-ID fast path (auto-IDs skip the version lookup), a modest per-doc indexing cost. This is the deliberate trade — bounded overhead for exactly-stored-once under retry — and it is worth it precisely in the topology where retries happen (Kafka at-least-once). For the async/direct-ingest path it can remain opt-in.

This is delivered as its own milestone and its own PR (M4 below): it is self-contained, independently valuable (it also removes duplicate-on-retry for the async path), changes observable wire behavior (dedup, snapshot-visible), and carries the data-stream `op_type`/409 subtlety — all reasons to review it in isolation rather than fold it into the sync-wiring milestone. It should land **before** `write_mode: sync` becomes a default, so that at-least-once is idempotent the moment it is on.

### 4.8 Autonomous poison-pill handling (dead-letter, no operator)

A **poison document** fails deterministically on every attempt (mapping conflict, field-type clash, oversized field). Under synchronous at-least-once with `message_marking.after: true`, failing the batch holds the offset — correct for a *transient* failure, fatal for a *poison* one: the partition re-delivers the same poison forever (§4.6). The system must resolve this **without a human**, because at scale a stalled partition is an outage, not a ticket. The design:

1. **Classify each item's `_bulk` result.** The writer already parses per-item status (M2). Split failures into **transient** — `429` (backpressure), `503`, connection/timeout, whole-target-unavailable — and **terminal** — `400`, mapping/parse/validation rejections, and any status that will not change on replay.
2. **Transient → real error → retry the whole batch.** The offset is held; `exporterhelper`/Kafka re-delivery re-sends; idempotent `_id` (§4.7) means the already-durable items are no-ops, so only the genuinely-missing items are (re)written. This is the normal back-pressure path.
3. **Terminal → dead-letter, then advance.** Route each terminal item to a **dead-letter sink** (a separate `*-dead-letter` index/data stream, or a dead-letter Kafka topic) with the rejection reason attached. Once the poison items are safely in the dead-letter sink, the batch is **complete** — the good items are durable, the poison items are preserved out-of-band — so `WriteTraces` returns `nil` and the offset advances. The partition never blocks; no data is silently dropped; the poison is inspectable later.
4. **If the dead-letter write itself fails, that is a transient failure of the sink** → return an error and hold the offset (don't advance on unconfirmed dead-lettering). The dead-letter sink must be at least as available as the primary; a separate index on the same cluster, or a Kafka topic, satisfies this.
5. **Observability, not intervention.** Emit a `dead_lettered` counter (by reason) and a log per poison doc so humans are *notified* asynchronously — but the pipeline has already recovered on its own. Optionally cap dead-letter volume with an alarm to catch a systemic mapping regression (a flood of terminal failures) versus the occasional bad document.

This preserves the two invariants together: **no silent loss** (poison goes to the dead-letter sink, not `/dev/null`) and **no head-of-line blocking** (the live partition always progresses). It depends on §4.7 (so batch retry for the transient case doesn't duplicate) and is delivered as its own milestone (M7 below), since a durable, autonomous dead-letter path is a substantial, independently-testable component.

---

## 5. Building on RFC 0006 and RFC 0004

This work is small and lands cleanly on top of the client foundation [RFC 0006](./0006-unified-elasticsearch-client.md) is building:

- **RFC 0006 M6 has merged** ([#8944](https://github.com/jaegertracing/jaeger/pull/8944)): the owned `esclient.BulkWriter` and the bounded bulk indexer over `esutil.BulkIndexer` exist and carry all span/service writes; `olivere` is gone from the write path (it remains only for the reader/control-plane, being migrated by later 0006 milestones). M6 delivered the **async** half; this RFC adds the **synchronous** half over the same `esclient` transport. `esutil.BulkIndexer` does expose a blocking `Flush`, but it delivers per-item results only through `OnSuccess`/`OnFailure` callbacks over a shared, worker-pooled buffer (`Flush` itself returns only transport-level errors, and drains *all* concurrent callers' queued items), so it yields no clean synchronous per-batch verdict. The sync path is therefore a **new, distinct primitive** — a blocking bulk on `esclient` (e.g. `Bulk(ctx, items) (BulkResult, error)`), reusing 0006's owned request/response types and the shared transport pool. (It re-implements esutil's small NDJSON encoder — the cheap part — precisely to avoid esutil's callback/shared-buffer result model.) It is **not** a method on the existing async `BulkWriter`; the two are peers selected by `write_mode`. Concretely: `SpanWriter` holds either the async `BulkWriter` (today) or a synchronous bulk client, chosen at construction from config.
- **RFC 0004 (Data Streams)** already changes the write op-type (`index`→`create`) and adds `@timestamp`. The sync writer inherits both transparently — it writes whatever documents `SpanWriter` produces (which already emits `esclient.BulkItem`s with the right `OpType`). The service-doc cache ordering (§4.3) reuses the same "cache after durable write" discipline data streams' trace-timestamp index needs.

There is no conflict; there is shared surface. Treat "synchronous write mode" as a peer of M6's async `BulkWriter` on the same `esclient`, with the contract/Kafka fix (this RFC) as the motivating requirement for the synchronous primitive.

---

## 6. Migration & backward compatibility

- **Default unchanged at introduction.** `write_mode: async` remains the default in the alpha stage of the `es.write.synchronous` feature gate (§4.5); existing deployments see no behavior change.
- **Opt-in, then default-flip via feature gate.** Operators opt into `write_mode: sync` **together with** `message_marking.after: true` and a blocking pipeline batcher, removing the fire-and-forget `batch` processor (§4.2/§4.5). Over the gate's alpha→beta→stable lifecycle the *default* moves to `sync`, giving operators a release window to adopt the pipeline shape before it becomes default.
- **No schema/on-disk change.** Documents, indices, mappings, and read paths are identical. This is purely a write-*mechanism* change.
- **Rollback.** Set `write_mode: async` explicitly (overrides the gate). No data migration.

---

## 7. Open questions

**Q1 — Exporter batcher (now) vs. receiver-level batching (later)?**
The blocking-and-fan-out semantics of `queue.wait_for_result` + `queue.batch` are **confirmed in the `exporterhelper` v0.155.0 source and its tests** (§4.2), so option 1 is safe to ship as config-only. The genuinely open item is whether to *also* invest in receiver-level per-partition batching (§4.2 option 2) — simpler offsets, but needs an upstream contrib change or a Jaeger receiver wrapper. Recommendation: ship option 1; open an upstream issue for option 2 and adopt it if accepted. Pin the `exporterhelper` version, since the batcher internals are `internal/` and could change.

**Q2 — Startup validation of the sync + batch-processor misconfiguration.**
`write_mode: sync` with a fire-and-forget `batch` processor still in the pipeline silently defeats the guarantee (§4.5). Can the exporter detect this at startup and warn/fail? The exporter does not see the full pipeline graph, so detection may not be feasible from inside the component; the fallback is prominent documentation. Recommendation: investigate a collector-level check; ship docs regardless.

**Q3 — Interface doc for `WriteTraces` async allowance.**
The `tracestore.Writer` doc comment should state explicitly whether an implementation may be asynchronous. Today the comment implies synchronous semantics that only Cassandra/ClickHouse honor. Recommendation: document that a *conformant* `WriteTraces` returns write errors synchronously, and that `async` mode is a deliberate, documented deviation trading the guarantee for throughput on unbatched ingest (§3.1) — not an accident.

**Q4 — Deterministic `_id` for span dedup? → Resolved: in scope (§4.7).**
Originally deferred to RFC 0004. Reconsidered: synchronous at-least-once makes retries routine, so idempotent writes are a prerequisite for production safety, not a nicety. Decision: adopt a **deterministic content-hash `_id`** for spans (matching Cassandra's `span_hash`, *not* `traceID+spanID+startTime` — see §4.7 for why the shared-span model requires a content hash), delivered as its own milestone/PR ahead of the default flip to `sync`. Remaining sub-question: the exact hash input (reuse Cassandra's gob-of-zeroed-span, or a cheaper canonical form) — settle during M4. The `op_type: create` 409-as-idempotent-success handling (§4.7) is the one non-obvious implementation point.

**Q6 — Dead-letter sink: separate index or Kafka topic? (§4.8)**
Autonomous poison-pill disposal (§4.8) needs a durable sink. A `*-dead-letter` index/data stream on the same cluster is simplest (no new dependency, reuses the ES client) but shares the cluster's fate; a dead-letter **Kafka topic** is more decoupled but reintroduces a Kafka dependency on the write side. Recommendation: default to a dead-letter index (available everywhere the writer already is), leave a Kafka-topic sink as a configurable alternative for Kafka-centric deployments; decide during M7.

**Q5 — Is `async` ever removed, or retained for direct ingest?**
The feature gate flips the *default* to `sync`, but async remains the better fit for unbatched direct ingest (§3.1). Two end-states: (a) at "stable", remove async entirely and require the blocking batcher even for direct ingest (accepting its `flush_timeout` latency); or (b) keep async as a permanent non-default option for the collector→storage path, only deprecating it for the Kafka ingester. Recommendation: decide at the beta→stable boundary once operational data exists; lean toward (b), since async is genuinely correct for that topology.

---

## 8. Implementation plan

The work is decomposed into independently shippable milestones, each leaving the tree green with no user-visible behavior change until the mode is explicitly opted into. Milestone status is tracked here (✅ Done, with the delivering PR) as work lands.

**Every milestone that introduces functional code must carry end-to-end coverage for that code *in the same milestone* — not defer it to a later validation step.** The `internal/storage/integration` suite runs against real ES 7–9 / OS 1–3 containers in CI, so a new `esclient` primitive is exercised there through the owned client, and a wired write-path change is validated by running that suite in the new mode. A milestone whose only tests are httptest mocks does not count as covered.

- **M1 — Core batch-write API. ✅ Done ([#8990](https://github.com/jaegertracing/jaeger/pull/8990)).** Replace the fire-and-forget `core.Writer.WriteSpan(startTime, *span)` with a batch, error-returning `WriteSpans(ctx, []dbmodel.Span) error` (§4.1) and route `TraceWriter.WriteTraces` through it; the async `SpanWriter` implements it by enqueuing each span into the existing `esutil.BulkIndexer` and returning `nil` (unchanged behavior — an async enqueue cannot fail synchronously). Clarify the `tracestore.Writer` doc comment on synchronous-error expectations and the documented async deviation (Q3). *Exit:* `WriteTraces` returns whatever `WriteSpans` returns; async behavior and wire format unchanged; the batch entry point that later milestones swap the sink under exists and is unit-tested.
- **M2 — Synchronous bulk primitive. ✅ Done ([#8992](https://github.com/jaegertracing/jaeger/pull/8992)).** Add a new blocking bulk writer on `esclient` (peer of M6's async `BulkWriter`, over the same transport — §5): one `_bulk` round-trip per size-bounded (`max_bytes`) chunk, parsing the response and returning transport + item-level errors from owned response types. *Exit:* byte-cap + item-error propagation proven by unit tests, **and** an ES/OS integration test (in the real ES 7–9 / OS 1–3 matrix) that round-trips documents to prove durability and forces a real item-level rejection to prove the error propagates — the primitive is exercised against a live backend the milestone it lands, not only via httptest.
- **M3 — `write_mode` config + feature gate.** Add `write_mode: async|sync` (explicit wins) and the `es.write.synchronous` feature gate governing the unset default (alpha: `async` — §4.5, §6); reuse `max_bytes` as chunk cap. *Exit:* config parse/validate + defaults + gate tests.
- **M4 — Deterministic span `_id` (idempotent writes) — §4.7.** Give each span a deterministic **content-hash** `_id` (matching Cassandra's `span_hash`, not `traceID+spanID+startTime`), so a re-sent identical span upserts rather than duplicating. Handle the `op_type: create` (data-stream) case where a duplicate `_id` returns 409 by treating a 409 on our own `_id` as idempotent success. Its own PR — self-contained, independently valuable (removes duplicate-on-retry for async too), and wire-observable. Lands before the default flips to `sync`. *Exit:* writing the same span twice yields exactly one document, proven by an ES/OS integration test on both `op_type: index` and `op_type: create`; request snapshots updated for the new `_id`.
- **M5 — Wire sync path.** `TraceWriter.WriteTraces` → `WriteSpans` → synchronous bulk; append service/operation docs into the same request; update the service cache only after success (§4.3). *Exit:* the full ES/OS trace-storage integration suite passes with `write_mode: sync` (parametrized alongside the async run) across the ES 7–9 / OS 1–3 matrix, so the wired write/read path is validated end-to-end in the new mode; plus a fault-injection integration test asserting a failing bulk makes `WriteTraces` return an error (and, on the ingester, the Kafka offset is **not** committed).
- **M6 — Blocking batcher (config, plus one validation check).** Provide/validate the ingester pipeline shape: `queue.wait_for_result: true` + `queue.batch` on the storage exporter, the pipeline `batch` processor removed, and the Kafka receiver's `message_marking.after: true`. The `exporterhelper` fan-out/blocking is already proven by its own tests (§4.2), so this step is mostly an example config + a Jaeger-side integration test. The one piece of new code: `jaeger_storage_exporter.start()` hard-fails when `write_mode: sync` and `queue.batch.max_size > bulk_processing.max_bytes` (§4.5). *Exit:* an ingester config that nacks every message in a failed batch and advances no offset; a mismatched `max_size`/`max_bytes` config fails startup.
- **M7 — Autonomous poison-pill handling (dead-letter) — §4.8.** Classify per-item `_bulk` failures into transient (retry the batch) vs. terminal (poison); route terminal items to a durable dead-letter sink (default: a `*-dead-letter` index) with the reason attached, then advance the offset. Emit a `dead_lettered` counter + log. This is what makes `write_mode: sync` production-safe: no head-of-line blocking and no silent loss. Required before the default flips to `sync` (beta). *Exit:* an integration test injecting a permanently-rejected document (e.g. a mapping conflict) shows the poison doc landing in the dead-letter sink, the good docs durable, the batch returning success, and the offset advancing — the partition never stalls; a transient failure still holds the offset.
- **M8 — Ingester validation (at-least-once end-to-end).** End-to-end test on the Kafka path with the sync writer + deterministic `_id` + blocking batcher + poison dead-lettering + `message_marking.after: true`: kill/reject ES mid-stream, assert no offset advance and full recovery on ES return with **no duplicates** (idempotent `_id`) and **no stall** (poison dead-lettered). *Exit:* at-least-once demonstrated end-to-end, autonomously.
- **M9 — Docs.** Document `write_mode` **and** the co-required settings (`message_marking.after: true`, blocking batcher, no `batch` processor — §4.5), the deterministic-`_id`/idempotency behavior (§4.7), the dead-letter sink and its config (§4.8), upstream-sizing guidance, the durability-vs-searchability note (§2.2), and the acking rationale (§4.6). *Exit:* configuration guide updated.
- **M10 — (Optional, upstream) Receiver-level batching.** Open a contrib issue/PR to let the Kafka receiver coalesce per-partition records into one `ConsumeTraces` (§4.2 option 2). Independent of M1–M9; adopt if accepted.

---

## 9. References

- [Issue #8476 — v2 Elasticsearch WriteTraces cannot propagate write failures](https://github.com/jaegertracing/jaeger/issues/8476)
- [PR #8281 — synchronous bulk in v2 (closed)](https://github.com/jaegertracing/jaeger/pull/8281)
- [PR #8651 — surface async bulk-write errors (draft)](https://github.com/jaegertracing/jaeger/pull/8651)
- [Issue #2192 — unbounded bulk memory / 413](https://github.com/jaegertracing/jaeger/issues/2192)
- [Issue #7612 — Replace olivere/elastic driver](https://github.com/jaegertracing/jaeger/issues/7612)
- **History:** [PR #47 — v1 `spanstore.Writer` interface (`WriteSpan`, one span)](https://github.com/jaegertracing/jaeger/pull/47) (`db6587b9`, 2017-02-15) · [PR #200 — original synchronous ES writer](https://github.com/jaegertracing/jaeger/pull/200) (`f3717836`, 2017-06-19) · [PR #656 — "Use elasticsearch bulk API" (the sync→async switch)](https://github.com/jaegertracing/jaeger/pull/656) (`e52ecffb`, 2018-01-23) · [PR #6090 — `BulkProcessing` config struct](https://github.com/jaegertracing/jaeger/pull/6090) · [PR #8944 — M6, owned `esutil.BulkIndexer`](https://github.com/jaegertracing/jaeger/pull/8944)
- [Cassandra v2 trace writer (`internal/storage/v2/cassandra/tracestore/writer.go`) — synchronous per-span, `errors.Join`](../../internal/storage/v2/cassandra/tracestore/writer.go)
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
