gRPC Remote Storage Plugins
===========================

Update (May 2024): as of v1.58, Jaeger will no longer support grpc-sidecar plugin model. Only gRPC Remote Storage API is suppored.

In order to support an ecosystem of different backend storage implementations, Jaeger supports a gRPC-based Remote Strorage API. The custom backend storage solutions need to implement `storage_v1` gRPC interfaces defined in `internal/storage/v1/grpc/proto/`.

```
+----------------------------------+                  +-----------------------------+
|                                  |                  |                             |
|                  +-------------+ |       gRPC       | +-------------+             |
|                  |             | |                  | |             |             |
| jaeger-component | grpc-client +----------------------> grpc-server | custom impl |
|                  |             | |                  | |             |             |
|                  +-------------+ |                  | +-------------+             |
|                                  |                  |                             |
+----------------------------------+                  +-----------------------------+

    Jaeger official components                         Custom backend implementation
```

Implementing a plugin
----------------------

Although the instructions below are limited to Go, plugins can be implemented any language. Languages other than
Go would implement a gRPC server using the `storage_v1` proto interfaces. The `proto` file can be found in `internal/storage/v1/grpc/proto/`.
To generate the bindings for your language you would use `protoc` with the appropriate `xx_out=` flag. This is detailed
in the [protobuf documentation](https://developers.google.com/protocol-buffers/docs/tutorials) and you can see an example of
how it is done for Go in the top level Jaeger `Makefile`.

The easiest way to generate the gRPC storage plugin bindings is to use [Docker Protobuf](https://github.com/jaegertracing/docker-protobuf/) which is a lightweight `protoc` Docker image containing the dependencies needed to generate code for multiple languages. For example, one can generate bindings for C# on Windows with Docker for Windows using the following steps:
1. First clone the Jaeger github repo to a folder (e.g. `c:\source\repos\jaeger`):
```
$ mkdir c:\source\repos\jaeger
$ cd c:\source\repos\jaeger
$ git clone https://github.com/jaegertracing/jaeger.git c:\source\repos\jaeger
```
2. Initialize the Jaeger repo submodules (this pulls in code from other Jaeger repositories that are needed):
```
$ git submodule update --init --recursive
```
3. Then execute the following Docker command which mounts the local directory `c:\source\repos\jaeger` to the directory `/jaeger` in the Docker container and then executes the `jaegertracing/protobuf:0.2.0` command. This will create a file called `Storage.cs` in your local Windows folder `c:\source\repos\jaeger\code` containing the gRPC Storage API bindings.
```
$ docker run --rm -u 1000 -v/c/source/repos/jaeger:/jaeger -w/jaeger \
    jaegertracing/protobuf:0.2.0 "-I/jaeger -Iidl/proto/api_v2 -I/usr/include/github.com/gogo/protobuf -Iinternal/storage/v1/grpc/proto --csharp_out=/jaeger/code internal/storage/v1/grpc/proto/storage.proto"
```

An example of a Go binary that implements Remote Storage API can be found in `cmd/remote-storage`. That specific binary does not implement a custom backend, instead it supports the same backend implementations as available directly in Jaeger, but it makes them accessible via a Remote Storage API (and is being used in the integration tests).

The API consists of several gRPC services:
  * `SpanReaderPlugin` - used for querying the data
  * `SpanWriterPlugin` - used for writing data
  * (optional) `StreamingSpanWriterPlugin` - allows more efficient transmission
  * (optional) `ArchiveSpanWriterPlugin` and `ArchiveSpanReaderPlugin` - to support archiving storage
  * (optional) `DependenciesReaderPlugin` - for reading service dependencies
  * (optional) `PluginCapabilities` - can be interrogated to find out which services an implementation supports

The API supports writing spans via gRPC stream, instead of unary messages. Streaming writes can improve throughput and decrease CPU load (see benchmarks in Issue #3636). The backend needs to implement `StreamingSpanWriterPlugin` service and indicate support via the `streamingSpanWriter` flag in the `Capabilities` response.

Note that using the streaming spanWriter may make the collector's `save_by_svr` metric inaccurate, in which case users will need to pay attention to the metrics provided by the plugin.

Certifying compliance
---------------
A plugin implementation shall verify it's correctness with Jaeger storage protocol by running the storage integration tests from [integration package](https://github.com/jaegertracing/jaeger/blob/main/plugin/storage/integration/integration.go#L397).

```golang
import (
	jaeger_integration_tests "github.com/jaegertracing/jaeger/plugin/storage/integration"
)

func TestJaegerStorageIntegration(t *testing.T) {
        ...
	si := jaeger_integration_tests.StorageIntegration{
		SpanReader: createSpanReader(),
		SpanWriter: createSpanWriter(),
		CleanUp: func() error { ... },
		Refresh: func() error { ... },
		SkipList: []string {  // Skip any unsupported tests
		},
	}
	// Runs all storage integration tests.
	si.IntegrationTestAll(t)
}
```
For more details, refer to one of the following implementations.

1. [jaeger-clickhouse](https://github.com/jaegertracing/jaeger-clickhouse/blob/798c568c1e1a345536f35692fca71196a796811e/integration/grpc_test.go#L88-L107)
2. [Timescale DB via Promscale](https://github.com/timescale/promscale/blob/ccde8accf5205450891e805e23566d9a11dbf8d3/pkg/tests/end_to_end_tests/jaeger_store_integration_test.go#L79-L97)

Tracing
-------

Jaeger requests to the backend implementation will include standard OTEL tracing headers. The implementation may choose to participate in those traces to allow end to end visibility of Jaeger's own operations (typically only enabled for read requests,
as tracing write requests results in traces for traces and may cause infinite loops).


Bearer token propagation from the UI
------------------------------------
When using `--query.bearer-token-propagation=true`, the bearer token will be properly passed on to the gRPC plugin server. To get access to the bearer token in your plugin, use a method similar to:

```golang
import (
    // ... other imports
    "fmt"
    "google.golang.org/grpc/metadata"

    "github.com/jaegertracing/jaeger/internal/storage/v1/grpc"
)

// ... spanReader type declared here

func (r *spanReader) extractBearerToken(ctx context.Context) (string, bool) {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		values := md.Get(grpc.BearerTokenKey)
		if len(values) > 0 {
			return values[0], true
		}
	}
	return "", false
}

// ... spanReader interface implementation

func (r *spanReader) GetServices(ctx context.Context) ([]string, error) {
    str, ok := r.extractBearerToken(ctx)
    fmt.Println(fmt.Sprintf("spanReader.GetServices: bearer-token: '%s', wasGiven: '%t'" str, ok))
    // ...
}
```
