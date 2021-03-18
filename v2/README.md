# Jaeger v2

The Jaeger v2 code base is a collection of libraries that are ultimately assembled as an [OpenTelemetry Collector](https://github.com/open-telemetry/opentelemetry-collector) distribution using the [OpenTelemetry Collector Builder](https://github.com/open-telemetry/opentelemetry-collector-builder).

The binaries are described in the [`manifests`](./manifests) directory, and can be built with:

```console
$ make build
```

Individual components can be built with:

```console
$ make build-agent
```

The builder is a pre-requisite for this, and should be installed automatically when it's not available.

## Release

The v2 is released using [GoReleaser](https://goreleaser.com/). Just create a new tag starting with `v2`, such as `v2.0.0-alpha1` and GoReleaser will take it from there. The outputs are:

- an optimized binary for each component, built using the sources produced by `make build`
- a non-optimized binary, for debugging purposes
- a set of container images for each of the binaries above, for multiple platforms, published at both DockerHub and Quay.io
