# memstore-plugin

This package builds a binary that can be used as an example of a sidecar storage plugin.

Note that Jaeger now supports remote storages via gRPC API, so using plugins is discouraged.
For example, `memorystore` can be used as a remote backend (https://github.com/jaegertracing/jaeger/issues/3835).
