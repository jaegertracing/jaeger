#Getting Started with Jaeger

We're working hard on making all backend components of Jaeger open
source. Please watch the GitHub repository
<https://github.com/uber/jaeger> for updates.

Jaeger client libraries are already open source and for now can be used
with Zipkin backend. You can see a HOW-TO example in the blog post
[Tracing HTTP request latency in Go with
OpenTracing](https://medium.com/@YuriShkuro/tracing-http-request-latency-in-go-with-opentracing-7cc1282a100a).

##Running Jaeger in Docker Container

Coming soon...

##Tracing a Sample Application
**Hot R.O.D. - Rides on Demand**

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
    -   See [Running Jaeger backend in docker](#running-jaeger-in-docker-container)

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

