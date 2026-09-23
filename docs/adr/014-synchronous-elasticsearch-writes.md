# ADR-014: Synchronous Elasticsearch/OpenSearch Writes and Lossless Pipelines

* **Status**: Implemented — graduated from [RFC 0007](../rfc/0007-synchronous-elasticsearch-writes.md)
* **Date**: 2026-09-22

## Context

The `tracestore.Writer` contract says `WriteTraces` returns an error when spans were not persisted. The Elasticsearch/OpenSearch writer did not honor it: spans went into a client-side bulk buffer and the call returned before anything reached the backend, so a failed flush was logged and lost while every component upstream believed the spans were stored. Behind a Kafka ingester this turned a backend outage into silent data loss, because the receiver committed offsets for records the storage never wrote.

[RFC 0007](../rfc/0007-synchronous-elasticsearch-writes.md) analyzes the problem and lays out the design; it was delivered across milestones M1–M6 (issue [#8476](https://github.com/jaegertracing/jaeger/issues/8476)). **This ADR records the resulting architecture and the pipeline configurations that make it lossless.** The RFC holds the motivation, the alternatives, and the milestone history.

The implementation lives in:

* [`internal/storage/elasticsearch/esclient/sync_bulk.go`](../../internal/storage/elasticsearch/esclient/sync_bulk.go) — the blocking `_bulk` writer and the classification of per-item failures
* [`internal/storage/v2/elasticsearch/tracestore/core/writer.go`](../../internal/storage/v2/elasticsearch/tracestore/core/writer.go) — the batch write, the service cache, and the attribution of rejected items to spans
* [`internal/storage/v2/api/tracestore/writer.go`](../../internal/storage/v2/api/tracestore/writer.go) — `RejectedSpansError`, the storage-agnostic report of terminal rejections
* [`cmd/jaeger/internal/connectors/storagewriterconnector/`](../../cmd/jaeger/internal/connectors/storagewriterconnector/) — the `jaeger_storage_writer` connector that feeds a dead-letter pipeline
* [`cmd/jaeger/config-kafka-ingester-sync.yaml`](../../cmd/jaeger/config-kafka-ingester-sync.yaml) and [`cmd/jaeger/config-kafka-ingester-dead-letter.yaml`](../../cmd/jaeger/config-kafka-ingester-dead-letter.yaml) — the ingester configurations the Kafka end-to-end tests run against

## Decision

The Elasticsearch/OpenSearch writer has two modes, selected by `write_mode`. The default `async` keeps the buffered behavior. `sync` writes each batch handed to the storage as one blocking `_bulk` request and returns an error when any span was not persisted. Every span carries a deterministic content-hash `_id`, so a retried batch overwrites rather than duplicates. In sync mode, per-item `_bulk` failures are classified into transient and terminal; a transient failure fails the batch, and a terminal one is handled according to `poison_pill_handling` (`fail` or `drop`) or, when the write goes through the `jaeger_storage_writer` connector, forwarded to a dead-letter pipeline.

The storage error is only half of the guarantee. **The pipeline between the receiver and the storage must block on the write result**, which means no `batch` processor and either no exporter queue or a queue with `wait_for_result: true`. Jaeger ships the ingester configurations that satisfy this and documents the collector one; it does not pick the mode for the operator, because both the write mode and the pipeline shape are the operator's settings.

## Architecture

### The synchronous writer

`esclient.SyncBulkWriter` implements the same `esclient.BatchWriter` interface as the asynchronous `esutil.BulkIndexer`, so `core.Writer.WriteSpans` assembles a batch's documents once and the factory chooses the sink from `write_mode`. The synchronous writer sends the documents in chunks bounded by `bulk_processing.max_bytes` (5 MB when unset), each chunk one `_bulk` round trip, and parses the per-item results of every response. In sync mode the other `bulk_processing` settings, `flush_interval` and `workers`, have no effect.

Elasticsearch acknowledges a `_bulk` request only after the translog is committed on the primary and every in-sync replica, so a `2xx` item means the document is durable. It does not mean the document is searchable, which follows the index refresh interval; the writer does not request a refresh, because forcing one would cost indexing throughput for no durability benefit.

### Idempotent documents

Every span document's `_id` is a hash of the span's content, computed as Cassandra computes `span_hash`, in both write modes. A re-sent span produces the same `_id` and overwrites the existing document under `op_type: index`. The key is a content hash rather than `traceID+spanID+startTime` because the shared-span model allows a client span and a server span to share those three fields. The cost is that a client-supplied `_id` disables the auto-id fast path of Elasticsearch.

### Service and operation documents

The writer keeps an in-memory cache of the service and operation pairs it has already written, so each pair is sent once per process. The cache is updated only after the batch that carried the pair has settled: a successful write, or a write whose only failures were terminal and were attributed to spans. A batch with a transient failure leaves the cache untouched, so the pairs are re-sent with the retry.

### Poison pills

A poison pill is a document the backend rejects identically on every attempt, such as a span whose attribute type conflicts with the index mapping. The writer separates per-item failures into transient (`429`, `503`, connection and timeout errors) and terminal (`400` and other mapping or validation rejections). A transient failure returns an error for the whole batch. A terminal one is disposed of according to a decision that is deliberately split between two components:

* **The writer decides whether to report or discard.** `poison_pill_handling: fail`, the default, returns the rejection as an error and holds whatever is upstream. `drop` discards the rejected documents, logs a sample of the reasons, and completes the batch. There is no third enum value.
* **The pipeline decides what a report means.** Behind `jaeger_storage_exporter` a reported rejection is a failed batch. Behind the `jaeger_storage_writer` connector it is a set of spans to forward to another pipeline.

In `fail` mode the writer returns a `*tracestore.RejectedSpansError` that names every terminally rejected span by trace id and span id with the backend's reason, and says whether transient failures also occurred and whether any rejected document could not be attributed to a span. The type lives in the storage API rather than in the Elasticsearch packages so that the connector depends on neither.

### The dead-letter connector

`jaeger_storage_writer` is a traces-to-traces connector that replaces `jaeger_storage_exporter` in the pipeline. It embeds the same `exporterhelper` traces pipeline the exporter is built on, with its own write-and-forward function as the push step, so it takes the exporter's `queue` and `retry_on_failure` settings unchanged; an enabled `queue` must set `wait_for_result: true`, and the connector rejects a configuration that does not. When the writer returns a `RejectedSpansError` with no transient failures and no unattributed documents, the connector copies exactly the named spans, sets `jaeger.storage.rejection_reason` on each copy, and sends them to its output pipeline, which ends in any standard exporter. If that exporter returns an error, the connector returns it, so the batch is retried and the offset held. The connector logs each forwarded span at warning level and counts them in `jaeger_storage_writer_dead_letter_spans`; the counter has no reason label because Elasticsearch reasons embed document ids.

The dead-letter exporter must deliver synchronously, so its `sending_queue` is disabled, and it must refuse what it does not keep: the stock `otlp` receiver accepts a request whole or rejects it, and a `kafka` exporter acknowledges only what the broker stored, while an `otlphttp` exporter treats an OTLP partial-success response as success.

### What makes a pipeline lossless

A write error is useful only when it reaches a component that can act on it. Two collector components acknowledge spans before the storage has written them, and either one turns `write_mode: sync` back into fire-and-forget. The findings below are from the collector sources at v0.160.0.

* **The `batch` processor.** `ConsumeTraces` pushes the data onto a channel and returns nil. A shard goroutine exports the batch later; when that fails, the processor logs `Sender failed` and drops the batch, and no error is returned to the receiver. Jaeger's sample collector configurations, `config.yaml` and `config-elasticsearch.yaml`, use `processors: [batch]` and are therefore lossy.
* **The exporter queue without `wait_for_result`.** With a `queue` configured, `exporterhelper` enqueues the request, strips cancellation from its context, and returns nil. With `wait_for_result: true` the caller blocks on a done channel and receives the write's result. Without a `queue` block the exporter is synchronous: the caller's request goes through the retry sender to the push function, and its result is returned directly. `jaeger_storage_exporter` ships with no queue and with retries disabled.

The blocking queue is also the batching mechanism. `partitionBatcher` merges queued requests with `MergeSplit` up to `batch.max_size`, exports the merged request once, and `multiDone` fans that one result back to every request that contributed to it. The pipeline therefore gets both properties at once: Elasticsearch sees a large `_bulk` request, and every caller still learns whether its spans were written. `batch.flush_timeout` bounds the latency this adds on a quiet stream. Keeping `batch.max_size` at or below `bulk_processing.max_bytes` keeps each batch a single `_bulk` request.

Once the error reaches the receiver, the receiver's behavior decides the guarantee:

* **The OTLP receiver** converts a non-permanent error to gRPC `Unavailable`, which the HTTP handler maps to `503`; both are retryable per the OTLP specification, so an SDK or agent keeps the batch in its own buffer, backs off, and retries. Jaeger reports every storage error as non-permanent, including a terminal rejection under `poison_pill_handling: fail`, so a client would retry a poison span forever and everything queued behind it would wait. Direct ingest must therefore pair sync mode with `drop`, or with the dead-letter connector.
* **The Kafka receiver** marks a record's offset before processing unless `message_marking.after: true`, in which case it marks only after the pipeline returned. With `on_error: false` (the default) a failed record is not skipped; the receiver holds its offset and pauses the partition until the next consumer-group rebalance, which a single-replica ingester never sees. The exporter must therefore retry a failed batch itself, with `retry_on_failure.enabled: true` and `max_elapsed_time: 0`, so that the partition stays live and the offset is held by the retry rather than by a pause.

### Recommended configurations

Two topologies, two shapes. Both drop the `batch` processor and block on the write result through the exporter queue; they differ in who retries and how poison spans are disposed of.

| Setting | Direct ingest (OTLP clients → collector) | Kafka ingester |
|---|---|---|
| `processors` | `[]` | `[]` |
| `exporters` / `connectors` | `jaeger_storage_exporter` | `jaeger_storage_exporter`, or `jaeger_storage_writer` with a dead-letter pipeline |
| `queue.wait_for_result` | `true` | `true` |
| `queue.batch` | `sizer: bytes`, `max_size` ≤ `bulk_processing.max_bytes`, `flush_timeout` in the low hundreds of milliseconds | same |
| `retry_on_failure` | disabled: the client retries, and a collector-side retry would only hold the client's request open | `enabled: true`, `max_elapsed_time: 0` |
| receiver | `otlp` with defaults | `kafka` with `message_marking.after: true`, `on_error: false` |
| `write_mode` | `sync` | `sync` |
| `poison_pill_handling` | `drop` | `drop` behind the exporter, `fail` behind the connector |

For direct ingest, leaving `queue` out entirely is also lossless, with one `_bulk` request per client export request. The blocking queue is recommended because it merges the small requests of many clients into bulks the backend handles efficiently, at the cost of `flush_timeout` of added latency.

For the Kafka ingester, batch size is bounded by the partitions the ingester consumes: the receiver processes each partition serially and partitions concurrently, so at most one record per partition waits in the batcher at a time. Throughput scales with partitions and replicas, not with `batch.max_size`.

## Consequences

**Positive**

* `WriteTraces` honors the `tracestore.Writer` contract in sync mode, and the Kafka ingester can commit an offset only after the spans it covers are durable, verified end to end by the Kafka e2e suite with a fault-injecting proxy.
* Retries are idempotent in both modes because of the content-hash `_id`, which also removed duplicate-on-retry from the async path.
* Poison documents never stall a pipeline unattended: `drop` discards them, the connector preserves them out of band, and either way the batch completes.
* Direct-ingest clients receive a retryable OTLP error on a storage outage, which lets SDK and agent buffers provide back-pressure and retry instead of the collector absorbing the loss.
* The dead-letter sink is any standard exporter; Jaeger carries no sink code.

**Negative / trade-offs**

* The guarantee depends on four settings on three components that must line up, and the exporter cannot see the pipeline graph, so a `batch` processor left in place is not detected at startup. The documentation carries that burden.
* Sync mode adds a `_bulk` round trip of latency to each batch, plus `flush_timeout` when the blocking batcher is used.
* Kafka batch size is capped by partition count. A worker-pool consumer that decouples the two (RFC 0007 M8) is not built.
* `write_mode` governs only the span writer; dependency and sampling writes remain asynchronous.
* The default stays `async`, so an operator has to opt in, and Jaeger's sample collector configurations remain lossy until they are changed.

## References

* [RFC 0007: Synchronous Elasticsearch/OpenSearch Writes](../rfc/0007-synchronous-elasticsearch-writes.md) — the proposal, alternatives, and milestone history (M1–M8).
* Issue [#8476](https://github.com/jaegertracing/jaeger/issues/8476).
* [ADR-012](012-unified-elasticsearch-client.md) — the `esclient` transport the synchronous writer runs on.
* [`storagewriterconnector/README.md`](../../cmd/jaeger/internal/connectors/storagewriterconnector/README.md) — the connector's configuration reference.
* OpenTelemetry Collector v0.160.0: `processor/batchprocessor/batch_processor.go`, `exporter/exporterhelper/internal/queue/memory_queue.go`, `exporter/exporterhelper/internal/queuebatch/partition_batcher.go`, `receiver/otlpreceiver/internal/errors/errors.go`; contrib `receiver/kafkareceiver/config.go`.
