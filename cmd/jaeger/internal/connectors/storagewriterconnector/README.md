# jaeger_storage_writer

This module implements a traces→traces `connector.Traces` that writes spans into a Jaeger trace storage, obtained from the [jaeger_storage](../../extension/jaegerstorage/) extension exactly as [jaeger_storage_exporter](../../exporters/storageexporter/) does, and re-emits the spans the storage rejected terminally onto its output pipeline. Wire that pipeline to any standard exporter to get a dead-letter queue for poison spans without a custom sink (RFC 0007 §4.8).

The connector runs the storage write inside the same `exporterhelper` pipeline as the exporter, so `queue` (including `wait_for_result` and `batch`) and `retry_on_failure` are configured the same way and mean the same thing. For the same reason its queue and send metrics are the exporter ones (`otelcol_exporter_queue_size`, `otelcol_exporter_sent_spans`, …) labelled `exporter="jaeger_storage_writer"`; the spans it dead-letters are counted by `jaeger_storage_writer_dead_lettered_spans`. Its outcome per batch:

- The write succeeds, or every rejected span was terminal and the dead-letter pipeline accepted them: the connector returns success, so a Kafka receiver with `message_marking.after: true` advances its offset.
- The write fails as a whole, some spans failed transiently, or the dead-letter pipeline rejected the spans: the connector returns the error, so the batch is retried and the offset held. Poison spans are dead-lettered only once a retry sees no transient failures, so a retry never dead-letters the same span twice.

## Configuration

The named `trace_storage` must report the spans it rejects: for Elasticsearch/OpenSearch that is `write_mode: sync` with `poison_pill_handling: fail`. Against any other storage the connector writes exactly like `jaeger_storage_exporter` and nothing ever reaches the dead-letter pipeline; under `drop` the writer logs each discarded document instead.

The dead-letter exporter must deliver synchronously (disable its `sending_queue`), otherwise it acknowledges a span the moment it is enqueued and the connector advances the offset before the sink has it.

```yaml
service:
  pipelines:
    traces:
      receivers: [kafka]
      processors: []
      exporters: [jaeger_storage_writer]
    traces/dead_letter:
      receivers: [jaeger_storage_writer]
      exporters: [kafka/dead_letter]

connectors:
  jaeger_storage_writer:
    trace_storage: some_storage
    retry_on_failure:
      enabled: true
      max_elapsed_time: 0
    queue:
      wait_for_result: true
      batch:
        flush_timeout: 200ms

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
```

See [`config-kafka-ingester-dead-letter.yaml`](../../../config-kafka-ingester-dead-letter.yaml) for the complete ingester configuration this module is exercised with in CI.
