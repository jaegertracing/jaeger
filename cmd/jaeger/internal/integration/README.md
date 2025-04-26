# Integration

The Jaeger v2 integration test is an extension of the existing `integration.StorageIntegration` designed to test the Jaeger-v2 OtelCol binary; currently, it only tests the span store. The existing tests at `internal/storage/integration` (also called "unit mode") test by writing and reading span data directly to the storage API. In contrast, these tests (or "e2e mode") read and write span data through the RPC client to the Jaeger-v2 OtelCol binary. E2E mode tests read from the jaeger_query extension and write to the receiver in OTLP formats. For details, see the [Architecture](#architecture) section below.

## Architecture

```mermaid
flowchart LR
    Test -->|writeSpan| SpanWriter
    SpanWriter --> RPCW[RPC_client]
    RPCW --> Receiver
    Receiver --> Exporter
    Exporter --> B(StorageBackend)
    Test -->|readSpan| SpanReader
    SpanReader --> RPCR[RPC_client]
    RPCR --> jaeger_query
    jaeger_query --> B

    subgraph Integration Test Executable
        Test
        SpanWriter
        SpanReader
        RPCW
        RPCR
    end

    subgraph jaeger-v2
        Receiver
        Exporter
        jaeger_query
    end
```

Integration tests require cleaning up the data in the storage between tests to produce independent results. This is achieved with a `storagecleaner` extension. The configuration for this extension is auto-injected into standard collector configs located in `/cmd/jaeger/`. The extension opens an HTTP endpoint (`POST /purge`) in the collector which retrieves the storage factory from the `jaegerstorage` extension and if the factory implements the `Purger` interface it calls the `purge()` function.

```mermaid
flowchart LR
    Receiver --> Processor
    Processor[...] --> Exporter
    JaegerStorageExension -.->|"(1) getStorage()"| Exporter
    Exporter -->|"(2) writeTrace()"| Storage

    e2e_test -->|"(1) POST /purge"| HTTP_endpoint
    JaegerStorageExension -.->|"(2) getStorage()"| HTTP_endpoint
    HTTP_endpoint -->|"(3) factory.(*Purger).Purge()"| Storage

    style e2e_test fill:blue,color:white
    style HTTP_endpoint fill:blue,color:white

    subgraph Jaeger Collector
        Receiver
        Processor
        Exporter

        subgraph JaegerStorageExension[Jaeger Storage Exension]
            Storage[(Trace
                     Storage)]
        end
        subgraph StorageCleanerExtension[Storage Cleaner Extension]
            HTTP_endpoint([HTTP
                           endpoint])
        end
    end
```

## Kafka Integration

The primary difference between the Kafka integration tests and other integration tests lies in the flow of data. In the standard tests, spans are written by the SpanWriter, sent through an RPC_client directly to a receiver, then to an exporter, and written to a storage backend. Spans are read by the SpanReader, which queries the jaeger_query process accessing the storage backend. In contrast, the Kafka tests introduce Kafka as an intermediary. Spans go from the SpanWriter through an RPC_client to an OTLP receiver in the Jaeger Collector, exported to Kafka, received by the Jaeger Ingester, and then stored. For details, see the [Architecture](#KafkaArchitecture) section below.

``` mermaid
flowchart LR
        Test -->|writeSpan| SpanWriter
        SpanWriter --> RPCW[RPC_client]
        RPCW --> OTLP_Receiver[Receiver]
        OTLP_Receiver --> CollectorExporter[Kafka Exporter]
        CollectorExporter --> Kafka[Kafka]
        Kafka --> IngesterReceiver[Kafka Receiver]
        IngesterReceiver --> IngesterExporter[Exporter]
        IngesterExporter --> StorageBackend[(In-Memory Store)]
        Test -->|readSpan| SpanReader
        SpanReader --> RPCR[RPC_client]
        RPCR --> QueryProcess[jaeger_query]
        StorageCleaner -->|purge| StorageBackend
        QueryProcess --> StorageBackend
        

        subgraph Integration_Test_Executable
            Test
            SpanWriter
            SpanReader
            RPCW
            RPCR
        end

        subgraph Jaeger Collector
            OTLP_Receiver
            CollectorExporter
        end

        subgraph Jaeger Ingester
            IngesterReceiver
            IngesterExporter
            QueryProcess
            StorageBackend
            StorageCleaner[Storage Cleaner Extension]
        end

        subgraph Kafka
            Topic
        end
```

## gRPC Integration Test
``` mermaid
flowchart LR

Test --> |writeSpan| SpanWriter
Test --> |HTTP/purge| PurgeEndpoint
SpanWriter --> |0.0.0.0:4316| OTLP_Receiver
OTLP_Receiver --> |write| Storage
Test --> |readSpan| SpanReader
SpanReader --> |0.0.0.0:17271| RemoteStorageAPI
RemoteStorageAPI --> |read| Storage
PurgeEndpoint --> |purge| Storage
subgraph Integration Test Executable
    Test
    SpanWriter
    SpanReader
end
subgraph Remote Storage Backend
    OTLP_Receiver[OTLP Receiver]
    Storage[(In-Memory Store)]
    subgraph remote_storage extension
        RemoteStorageAPI[gRPC Endpoint]
    end
    subgraph storage_cleaner extension
        PurgeEndpoint[Purge Endpoint]
    end
end
```

## Running tests locally

You can run integration tests locally with the following command:

```sh
STORAGE={STORAGE_NAME} make jaeger-v2-storage-integration-test
```

where the storage name can be one of the following:

* badger
* cassandra
* grpc
* kafka
* memory_v2
* query
