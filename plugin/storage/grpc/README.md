gRPC Storage Plugins
====================
gRPC Storage Plugins currently use the [Hashicorp go-plugin](https://github.com/hashicorp/go-plugin). This requires the
implementer of a plugin to develop the "server" side of the go-plugin system. At a high level this looks like:

```
+----------------------------------+                  +-----------------------------+
|                                  |                  |                             |
|                  +-------------+ |   unix-socket    | +-------------+             |
|                  |             | |                  | |             |             |
| jaeger-component | grpc-client +----------------------> grpc-server | plugin-impl |
|                  |             | |                  | |             |             |
|                  +-------------+ |                  | +-------------+             |
|                                  |                  |                             |
+----------------------------------+                  +-----------------------------+

       parent process                                        child sub-process
```

Implementing a plugin
----------------------

Although the instructions below are limited to Go, plugins can be implemented any language. Languages other than
Go would implement a gRPC server using the `storage_v1.proto` interfaces. The `proto` file can be found in `plugin/storage/grpc/proto/`.
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
3. Then execute the following Docker command which mounts the local directory `c:\source\repos\jaeger` to the directory `/jaeger` in the Docker container and then executes the `jaegertracing/protobuf:0.2.0` command. This will create a file called `Storage.cs` in your local Windows folder `c:\source\repos\jaeger\code` containing the gRPC Storage Plugin bindings.
```
$ docker run --rm -u 1000 -v/c/source/repos/jaeger:/jaeger -w/jaeger \
    jaegertracing/protobuf:0.2.0 "-I/jaeger -Iidl/proto/api_v2 -I/usr/include/github.com/gogo/protobuf -Iplugin/storage/grpc/proto --csharp_out=/jaeger/code plugin/storage/grpc/proto/storage.proto"
```

There are instructions on implementing a `go-plugin` server for non-Go languages in the 
[go-plugin non-go guide](https://github.com/hashicorp/go-plugin/blob/master/docs/guide-plugin-write-non-go.md).
Take note of the required [health check service](https://github.com/hashicorp/go-plugin/blob/master/docs/guide-plugin-write-non-go.md#3-add-the-grpc-health-checking-service).
  
A Go plugin is a standalone application which calls `grpc.Serve(&pluginServices)` in its `main` function, where the `grpc` package 
is `github.com/jaegertracing/jaeger/plugin/storage/grpc`.
 
```go
    package main

    import (
        "flag"
        "github.com/jaegertracing/jaeger/plugin/storage/grpc"
    )
    
    func main() {
        var configPath string
        flag.StringVar(&configPath, "config", "", "A path to the plugin's configuration file")
        flag.Parse()

        plugin := myStoragePlugin{}
        
        grpc.Serve(&shared.PluginServices{
			Store:        plugin,
			ArchiveStore: plugin,
		})
    }
```
 
Note that `grpc.Serve` is called as the final part of the main. This should be called after you have carried out any necessary
setup for your plugin, as once running Jaeger may start calling to read/write spans straight away. You could defer
setup until the first read/write but that could make the first operation slow and also lead to racing behaviours.

A plugin must implement the StoragePlugin interface of:

```go
type StoragePlugin interface {
   	SpanReader() spanstore.Reader
   	SpanWriter() spanstore.Writer
   	DependencyReader() dependencystore.Reader
}
```

As your plugin will be dependent on the protobuf implementation within Jaeger you will likely need to `vendor` your
dependencies, you can also use `go.mod` to achieve the same goal of pinning your plugin to a Jaeger point in time.

A simple plugin which uses the memstore storage implementation can be found in the `examples` directory of the top level
of the Jaeger project.

To support archive storage a plugin must implement the ArchiveStoragePlugin interface of:

```go
type ArchiveStoragePlugin interface {
	ArchiveSpanReader() spanstore.Reader
	ArchiveSpanWriter() spanstore.Writer
}
```

If you don't plan to implement archive storage simply do not fill `ArchiveStore` property of `shared.PluginServices`:

```go
grpc.Serve(&shared.PluginServices{
    Store: plugin,
})
```

Running with a plugin
---------------------
A plugin can be run using the `all-in-one` application within the top level `cmd` package of the Jaeger project. To do this
an environment variable must be set to tell the `all-in-one` application to use the gRPC plugin storage:
`export SPAN_STORAGE_TYPE="grpc-plugin"` 

Once this has been set then there are two command line flags that can be used to configure the plugin. The first is 
`--grpc-storage-plugin.binary` which is required and is the path to the plugin **binary**. The second is 
`--grpc-storage-plugin.configuration-file` which is optional and is the path to the configuration file which will be
provided to your plugin as a command line flag. This command line flag is `config`, as can be seen in the code sample
above. An example invocation would be:

```
./all-in-one --grpc-storage-plugin.binary=/path/to/my/plugin --grpc-storage-plugin.configuration-file=/path/to/my/config
```

As well as passing configuration values via the command line through the configuration file it is also possible to use
environment variables. When you invoke `all-in-one` any environment variables that have been set will also be accessible
from within your plugin, this is useful if using Docker.

Logging
-------
In order for Jaeger to include the log output from your plugin you need to use `hclog` (`"github.com/hashicorp/go-hclog"`).
The plugin framework will only include any log output created at the `WARN` or above levels. If you log output in this
way before calling `grpc.Serve` then it will still be included in the Jaeger output. 

An example logger instantiation could look like:
 
 ```
logger := hclog.New(&hclog.LoggerOptions{
    Level:      hclog.Warn,
    Name:       "my-jaeger-plugin",
    JSONFormat: true,
})
```

There are more logger options that can be used with `hclog` listed on [godoc](https://godoc.org/github.com/hashicorp/go-hclog#LoggerOptions).

Note: Setting the `Output` option to `os.Stdout` can confuse the `go-plugin` framework and lead it to consider the plugin
errored.

Bearer token propagation from the UI
------------------------------------
When using `--query.bearer-token-propagation=true`, the bearer token will be properly passed on to the gRPC plugin server. To get access to the bearer token in your plugin, use a method similar to:

```golang
import (
    // ... other imports
    "fmt"
    "github.com/jaegertracing/jaeger/storage/spanstore"
    "google.golang.org/grpc/metadata"
)

// ... spanReader type declared here

func (r *spanReader) extractBearerToken(ctx context.Context) (string, bool) {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		values := md.Get(spanstore.BearerTokenKey)
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
