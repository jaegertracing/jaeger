gRPC Storage Plugins
====================
gRPC Storage Plugins currently use the Hashicorp go-plugin. This requires the implementer of a plugin to develop the 
"server" side of the go-plugin system.

Implementing a plugin
----------------------

A plugin is a standalone application which calls `grpc.Serve(&plugin)` in its `main` function, where the `grpc` package 
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
        
        grpc.Serve(&plugin)
    }
```
 
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
If the plugin uses a `hclog` (`"github.com/hashicorp/go-hclog"`) logger then any logs created at the `WARN` or above level will be
included in the log output of the `all-in-one` application. Bear in mind that any logging performed before `grpc.Serve`
is called will not be included. As well as this `grpc.Serve` should likely be called at the end of your `main` function
as once running Jaeger may start calling to read/write spans straight away. This can mean that logging output during any
setup can be challenging.
