# Getting Started

## All in one Docker image

This image, designed for quick local testing, launches the Jaeger UI, collector, query, and agent, with an in memory storage component. 

The simplest way to start the all in one docker image is to use the pre-built image published to DockerHub.

```bash
docker run -d -p5775:5775/udp -p16686:16686 jaegertracing/all-in-one:latest
```

You can then navigate to `http://localhost:16686` to access the Jaeger UI. 

## Sample Application

### HotROD (Rides on Demand)

This is a demo application that consists of several microservices and
illustrates the use of the [OpenTracing API](http://opentracing.io).
A tutorial / walkthough is available: [Take OpenTracing for a HotROD ride][hotrod-tutorial].

It can be run standalone, but requires Jaeger backend to view the
traces.

#### Running

```bash
go get github.com/uber/jaeger
cd $GOPATH/src/github.com/uber/jaeger
make install_examples
cd examples/hotrod
go run ./main.go all
```

Then navigate to `http://localhost:8080`.


#### Features

-   Discover architecture of the whole system via data-driven dependency
    diagram.
-   View request time line & errors, understand how the app works.
-   Find sources of latency, lack of concurrency.
-   Highly contextualized logging.
-   Use baggage propagation to

    -   Diagnose inter-request contention (queueing).
    -   Attribute time spent in a service.

-   Use open source libraries with OpenTracing integration to get
    vendor-neutral instrumentation for free.

#### Prerequisites

-   You need Go 1.7 or higher installed on your machine.
-   Requires a [running Jaeger backend](#all-in-one-docker-image) to view the traces.

## Client Libraries

Look [here](client_libraries.md).

## Running Individual Jaeger Components
Individual Jaeger backend components can be run from source.
They all have their `main.go` in the `cmd` folder. For example, to run the `jaeger-agent`:

```bash
go get github.com/uber/jaeger
cd $GOPATH/src/github.com/uber/jaeger
make install
go run ./cmd/agent/main.go
```

In the near future we will provide individual Docker images for each component,
as well as the Kubernetes templates.

## Migrating from Zipkin

Jaeger's agent and collector can also accept [Zipkin](http://zipkin.io/) [Spans](https://github.com/openzipkin/zipkin-api/blob/master/thrift/zipkinCore.thrift#L381), and transform them to Jaeger's data model before storage. 

Both of them support this Zipkin [idl](https://github.com/uber/jaeger-idl/blob/master/thrift/zipkincore.thrift), and expose the `ZipkinCollector` service.
On the agent, `ZipkinCollector` is available on `UDP` port `5775`, and uses the `TBinaryProtocol`.


[hotrod-tutorial]: https://medium.com/@YuriShkuro/take-opentracing-for-a-hotrod-ride-f6e3141f7941
