Tracing a Sample Application
============================

Hot R.O.D. - Rides on Demand
----------------------------

<https://github.com/uber/jaeger/tree/master/examples/hotrod>

This is a demo application that consists of several microservices and
illustrates the use of the [OpenTracing API](http://opentracing.io).

It can be run standalone, but requires Jaeger backend to view the
traces.

### Features

-   Discover architecture of the whole system via data-driven dependency
    diagram
-   View request timeline & errors, understand how the app works
-   Find sources of latency, lack of concurrency
-   Highly contextualized logging
-   Use baggage propagation to

    -   Diagnose inter-request contention (queueing)
    -   Attribute time spent in a service

-   Use open source libraries with OpenTracing integration to get
    vendor-neutral instrumentation for free

### Prerequisites

-   You need Go 1.7 or higher installed on your machine.
-   Requires a running Jaeger backend to view the traces.
    -   See [Running Jaeger backend in docker](backend_in_docker.md)

### Installation

```shell 
go get github.com/uber/jaeger
make install_examples
```

### Running

```shell
cd examples/hotrod
go run ./main.go all
```

Then open <http://127.0.0.1:8080>
