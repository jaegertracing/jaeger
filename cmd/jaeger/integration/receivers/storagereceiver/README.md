# Storage Receiver

`storagereceiver` is a fake receiver that creates an artificial stream of traces by:

- repeatedly querying one of Jaeger storage backends for all traces (by service).
- tracking new traces / spans and passing them to the next component in the pipeline.

# Getting Started

The following settings are required:

- `trace_storage` (no default): name of a storage backend defined in `jaegerstorage` extension

The following settings can be optionally configured:

- `pull_interval` (default = 0s): The delay between each iteration of pulling traces.

```yaml
receivers:
  jaeger_storage_receiver:
    trace_storage: external-storage
    pull_interval: 0s
```
