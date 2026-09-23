# jaeger_storage_exporter (connector)

This module implements the connector form of `jaeger_storage_exporter`: a traces→traces `connector.Traces` that writes spans into a Jaeger trace storage, obtained from the [jaeger_storage](../../extension/jaegerstorage/) extension exactly as the [exporter](../../exporters/storageexporter/) does, and re-emits the spans the storage rejected terminally onto its output pipeline. The two share one component type. The collector resolves a pipeline entry as a connector only when it is declared under `connectors:`, so `exporters: { jaeger_storage_exporter: ... }` is the plain storage write and `connectors: { jaeger_storage_exporter: ... }` is the same write with a dead-letter output. Wire that pipeline to any standard exporter to get a dead-letter queue for poison spans without a custom sink (RFC 0007 §4.8).

The connector runs the storage write inside the same `exporterhelper` pipeline as the exporter, so its `queue` and `retry_on_failure` blocks are the exporter's and mean the same thing.

The `queue` block is not a buffer that decouples the receiver from the write: with `wait_for_result: true` it is the blocking batcher of RFC 0007 §4.2, which merges the one-record `ConsumeTraces` calls a Kafka receiver makes into one `_bulk` request and returns that request's result to every caller. An enabled `queue` must therefore set `wait_for_result: true`, and the connector rejects a configuration that does not, because a queue that acknowledges on enqueue would advance the receiver before the storage has the spans. Leaving `queue` out is also valid: each `ConsumeTraces` call then becomes one write.

The outcome of each batch:

- The write succeeds, or every rejected span was terminal and the dead-letter pipeline accepted them: the connector returns success, so a Kafka receiver with `message_marking.after: true` advances its offset.
- The write fails as a whole, some spans failed transiently, or the dead-letter pipeline rejected the spans: the connector returns the error, so the batch is retried and the offset held. Poison spans go to the dead-letter pipeline only once a retry sees no transient failures, so a retry never sends the same span there twice.

Each span sent to the dead-letter pipeline is a copy of the input span with the attribute `jaeger.storage.rejection_reason` holding the storage's reason, and is logged at warn level with its trace id, span id, and reason. The counter `jaeger_storage_exporter_dead_letter_spans` counts them; it carries no reason label because Elasticsearch reasons embed document ids and would make the label unbounded. The queue and send metrics are the exporter ones (`otelcol_exporter_queue_size`, `otelcol_exporter_sent_spans`, …) labelled `exporter="jaeger_storage_exporter"`.

## Configuration

The named `trace_storage` must report the spans it rejects through `tracestore.RejectedSpansError`: for Elasticsearch/OpenSearch that is `write_mode: sync` with `poison_pill_handling: fail`. Against any other storage the connector writes exactly like `jaeger_storage_exporter` and nothing ever reaches the dead-letter pipeline; under `drop` the writer logs each discarded document instead.

The dead-letter exporter must deliver synchronously (disable its `sending_queue`), otherwise it acknowledges a span the moment it is enqueued and the connector advances the offset before the sink has it. It must also refuse what it does not keep: `otlphttp` treats an OTLP partial-success response as success, so its endpoint has to accept a request whole or reject it, as the stock `otlp` receiver does, and a `kafka` exporter acknowledges only what the broker stored.

```yaml
service:
  pipelines:
    traces:
      receivers: [kafka]
      processors: []
      exporters: [jaeger_storage_exporter]
    traces/dead_letter:
      receivers: [jaeger_storage_exporter]
      exporters: [kafka/dead_letter]

connectors:
  jaeger_storage_exporter:
    trace_storage: some_storage
    retry_on_failure:
      enabled: true
      max_elapsed_time: 0
    queue:
      wait_for_result: true
      sizer: bytes
      num_consumers: 1
      queue_size: 104857600
      batch:
        sizer: bytes
        flush_timeout: 200ms
        min_size: 1048576  # flush at 1 MiB, or when flush_timeout elapses
        max_size: 4194304

extensions:
  jaeger_storage:
    backends:
      some_storage:
        elasticsearch:
          server_urls: [http://localhost:9200]
          write_mode: sync
          poison_pill_handling: fail

exporters:
  kafka/dead_letter:
    brokers: [localhost:9092]
    traces:
      topic: jaeger-spans-dead-letter
    sending_queue:
      enabled: false
    # The exporter sends a batch's rejected spans as one record. A record larger
    # than the producer's limit (1,000,000 bytes by default) fails on every retry
    # and holds the offset, so allow at least the connector's queue.batch.max_size
    # plus the reason attributes, and raise the broker's message.max.bytes to match.
    producer:
      max_message_bytes: 8388608
```

An omitted `queue` field takes its zero value, not the exporterhelper default, so a `queue` block must spell out its sizing as above.

See [`config-kafka-ingester-dead-letter.yaml`](../../../config-kafka-ingester-dead-letter.yaml) for the complete ingester configuration the Kafka end-to-end tests run against.
