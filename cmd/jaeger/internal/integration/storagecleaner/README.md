# storage_cleaner

This module implements an extension that allows purging the backend storage by making an HTTP POST request to it. 

The storage_cleaner extension is intended to be used only in tests, providing a way to clear the storage between test runs. Making a POST request to the exposed endpoint will delete all data in storage.


```mermaid
flowchart LR
    Receiver --> Processor
    Processor --> Exporter
    JaegerStorageExension -->|"(1) get storage"| Exporter
    Exporter -->|"(2) write trace"| Badger

    Badger_e2e_test -->|"(1) POST /purge"| HTTP_endpoint
    JaegerStorageExension -->|"(2) getStorage()"| HTTP_endpoint
    HTTP_endpoint -.->|"(3) storage.(*Badger).Purge()"| Badger

    subgraph Jaeger Collector
        Receiver
        Processor
        Exporter
        
        Badger
        BadgerCleanerExtension
        HTTP_endpoint
        subgraph JaegerStorageExension
            Badger
        end
        subgraph BadgerCleanerExtension
            HTTP_endpoint
        end
    end
```

# Getting Started

The following settings are required:

- `trace_storage` : name of a storage backend defined in `jaegerstorage` extension

```yaml
extensions:
  storage_cleaner:
    trace_storage: storage_name
```

