# tracegen

`tracegen` is a utility that can generate a steady flow of simple traces useful for performance tuning.
Traces are produced concurrently from one or more worker goroutines. Run with `-h` to see all cli flags.

The binary is available from the Releases page, as well as a Docker image:

```sh
$ docker run jaegertracing/jaeger-tracegen -service abcd -traces 10
```

The generator can be configured to export traces in different formats, via `-exporter` flag.
By default, the exporters send data to `localhost`. If running in a container, this refers
to the networking namespace of the container itself, so to export to another container,
the exporters need to be provided with appropriate location.
OTLP exporter accepts configuration via environment variables.
For more information about configuring OTLP exporter, see [OpenTelemetry Protocol Exporter](https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/protocol/exporter.md).

See example in the included [docker-compose](./docker-compose.yml) file.
