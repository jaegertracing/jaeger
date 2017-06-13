<img align="right" src="images/jaeger-vector.svg" width=400>

# Jaeger, a distributed tracing system

Welcome to Jaeger's documentation portal! Below, you'll find information for beginners and experienced Jaeger users. 

If you can't find what you are looking for, or have an issue not covered here, we'd love to hear from you either on [Github](https://github.com/uber/jaeger/issues), [Mailing List](https://groups.google.com/forum/#!forum/jaeger-tracing) or on [Gitter Channel](https://gitter.im/jaegertracing/Lobby).

## About
Jaeger, inspired by [Dapper][dapper] and [OpenZipkin](http://zipkin.io),
is a distributed tracing system released as open source by [Uber Technologies][ubeross].
It can be used for monitoring microservice-based architectures:

* Distributed context propagation
* Distributed transaction monitoring
* Root cause analysis
* Service dependency analysis
* Performance / latency optimization

Jaeger is written in Go, with [OpenTracing](http://opentracing.io/) compatible client libraries available in [Go](https://github.com/uber/jaeger-client-go), [Java](https://github.com/uber/jaeger-client-java), [Node](https://github.com/uber/jaeger-client-node) and [Python](https://github.com/uber/jaeger-client-python). It uses Cassandra for storage.

See also: [Evolving Distributed Tracing at Uber](https://eng.uber.com/distributed-tracing/) blog post.

## Features

  * [OpenTracing](http://opentracing.io/) compatible data model and instrumentation libraries 
    * in [Go](https://github.com/uber/jaeger-client-go), [Java](https://github.com/uber/jaeger-client-java), [Node](https://github.com/uber/jaeger-client-node) and [Python](https://github.com/uber/jaeger-client-python)
  * Backend components implemened in Go
  * React-based UI
  * Cassandra as persistent storage
  * Uses consistent upfront sampling with individual per service/endpoint probabilities

## Quick Start
See [running a docker all in one image](getting_started.md#all-in-one-docker-image).

## Screenshots
### Traces View
[![Traces View](images/traces-ss.png)](images/traces-ss.png)

### Trace Detail View
[![Detail View](images/trace-detail-ss.png)](images/trace-detail-ss.png)

## Related links
- [Evolving Distributed tracing At Uber Engineering](https://eng.uber.com/distributed-tracing/)
- [Tracing HTTP request latency in Go with OpenTracing](https://medium.com/opentracing/tracing-http-request-latency-in-go-with-opentracing-7cc1282a100a)

[dapper]: https://research.google.com/pubs/pub36356.html
[ubeross]: http://uber.github.io
