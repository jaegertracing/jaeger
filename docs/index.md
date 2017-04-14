<img align="right" src="images/jaeger-vector.svg" width=400>
#Jaeger, a distributed tracing system

Welcome to Jaeger's documentation portal! Below, you'll find information for beginners and experienced Jaeger users. 

If you can't find what you are looking for, or have an issue not covered here, we'd love to hear from you either on [Github](https://github.com/uber/jaeger/issues), or on our [Mailing List](https://groups.google.com/forum/#!forum/jaeger-tracing). 

##About
Jaeger, inspired by Google's [Dapper](https://research.google.com/pubs/pub36356.html), is a distributed tracing system used to monitor, profile, and troubleshoot microservices.

Jaeger is written in Go, with [OpenTracing](http://opentracing.io/) compatible client libraries available in [Go](https://github.com/uber/jaeger-client-go), [Java](https://github.com/uber/jaeger-client-java), [Node](https://github.com/uber/jaeger-client-node) and [Python](https://github.com/uber/jaeger-client-python). It uses Cassandra for storage.

##Quick Start
See [running a docker all in one image](getting_started.md#all-in-one-docker-image).

##Screenshots
###Traces View
[![Traces View](images/traces-ss.png)](images/traces-ss.png)

###Trace Detail View
[![Detail View](images/trace-detail-ss.png)](images/trace-detail-ss.png)

##Related links
- [Evolving Distributed tracing At Uber Engineering](https://eng.uber.com/distributed-tracing/)
- [Tracing HTTP request latency in Go with OpenTracing](https://medium.com/opentracing/tracing-http-request-latency-in-go-with-opentracing-7cc1282a100a)
