#Getting Started

##Local Installation
#All in one images
These images combine all the components of Jaeger with an in memory storage component, and are designed for quick local testing.

####Docker
The simplest way to start the all in one docker image is to use the pre-built image published to docker hub.

```bash
docker run jaegertracing/all-in-one -p <ui_port>:16686 -p <agent_port>:5775/udp
```

You can then navigate to `localhost:<local_port>` to access the Jaeger UI. 

##Tracing a Sample Application
####Hot R.O.D. - Rides on Demand

This is a demo application that consists of several microservices and
illustrates the use of the [OpenTracing API](http://opentracing.io). It's source is at the 
[examples](https://github.com/uber/jaeger/tree/master/examples/hotrod) folder. 

It can be run standalone, but requires Jaeger backend to view the
traces.

### Features

-   Discover architecture of the whole system via data-driven dependency
    diagram
-   View request time line & errors, understand how the app works
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
    -   See [Running Jaeger backend in docker](#docker)

### Running 

```bash
go get github.com/uber/jaeger
cd $GOPATH/src/github.com/uber/jaeger
make install_examples
cd examples/hotrod
go run ./main.go all
```

Then open `http://127.0.0.1:8080`

##Client Libraries
Jaeger client libraries are already open source and for now can be used
with Zipkin backend. You can see a HOW-TO example in the blog post
[Tracing HTTP request latency in Go with
OpenTracing](https://medium.com/@YuriShkuro/tracing-http-request-latency-in-go-with-opentracing-7cc1282a100a).

