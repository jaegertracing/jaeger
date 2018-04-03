<img align="right" width="290" height="290" src="http://jaeger.readthedocs.io/en/latest/images/jaeger-vector.svg">

[![Build Status][ci-img]][ci] [![Coverage Status][cov-img]][cov] [![Gitter chat][gitter-img]][gitter] [![OpenTracing-1.0][ot-badge]](http://opentracing.io) [![FOSSA Status](https://app.fossa.io/api/projects/git%2Bgithub.com%2Fjaegertracing%2Fjaeger.svg?type=shield)](https://app.fossa.io/projects/git%2Bgithub.com%2Fjaegertracing%2Fjaeger?ref=badge_shield) [![CII Best Practices](https://bestpractices.coreinfrastructure.org/projects/1273/badge)](https://bestpractices.coreinfrastructure.org/projects/1273)

# Jaeger - a Distributed Tracing System

Jaeger, inspired by [Dapper][dapper] and [OpenZipkin](http://zipkin.io),
is a distributed tracing system released as open source by [Uber Technologies][ubeross].
It can be used for monitoring microservices-based distributed systems:

  * Distributed context propagation
  * Distributed transaction monitoring
  * Root cause analysis
  * Service dependency analysis
  * Performance / latency optimization

See also:

  * Jaeger [documentation][doc] for getting started, operational details, and other information.
  * Blog post [Evolving Distributed Tracing at Uber](https://eng.uber.com/distributed-tracing/).
  * Tutorial / walkthrough [Take OpenTracing for a HotROD ride][hotrod-tutorial].

<img src="https://www.cncf.io/wp-content/uploads/2016/09/logo_cncf.png">

Jaeger is hosted by the [Cloud Native Computing Foundation](https://cncf.io) (CNCF). If you are a company that wants to help shape the evolution of technologies that are container-packaged, dynamically-scheduled and microservices-oriented, consider joining the CNCF. For details about who's involved and how Jaeger plays a role, read the CNCF [announcement](https://www.cncf.io/blog/2017/09/13/cncf-hosts-jaeger/).

## Features

### High Scalability

Jaeger backend is designed to have no single points of failure and to scale with the business needs.
For example, any given Jaeger installation at Uber is typically processing several billions of spans per day.

### Native support for OpenTracing

Jaeger backend, Web UI, and instrumentation libraries have been designed from ground up to support the OpenTracing standard.
  * Represent traces as directed acyclic graphs (not just trees) via [span references](https://github.com/opentracing/specification/blob/master/specification.md#references-between-spans)
  * Support strongly typed _span tags_ and _structured logs_
  * Support general distributed context propagation mechanism via _baggage_

### Multiple storage backends

Jaeger supports two popular open source NoSQL databases as trace storage backends: Cassandra 3.4+ and Elasticsearch 5.x/6.x.
There are ongoing community experiments using other databases, such as ScyllaDB, InfluxDB, Amazon DynamoDB. Jaeger also ships
with a simple in-memory storage for testing setups.

### Modern Web UI

Jaeger Web UI is implemented in Javascript using popular open source frameworks like React. Several performance
improvements have been released in v1.0 to allow the UI to efficiently deal with large volumes of data, and to display
traces with tens of thousands of spans (e.g. we tried a trace with 80,000 spans).

### Cloud Native Deployment

Jaeger backend is distributed as a collection of Docker images. The binaries support various configuration methods,
including command line options, environment variables, and configuration files in multiple formats (yaml, toml, etc.)
Deployment to Kubernetes clusters is assisted by [Kubernetes templates](https://github.com/jaegertracing/jaeger-kubernetes)
and a [Helm chart](https://github.com/kubernetes/charts/tree/master/incubator/jaeger).

### Observability

All Jaeger backend components expose [Prometheus](https://prometheus.io/) metrics by default (other metrics backends are
also supported). Logs are written to standard out using the structured logging library [zap](https://github.com/uber-go/zap).

### Backwards compatibility with Zipkin

Although we recommend instrumenting applications with OpenTracing API and binding to Jaeger client libraries to benefit
from advanced features not available elsewhere, if your organization has already invested in the instrumentation
using Zipkin libraries, you do not have to rewrite all that code. Jaeger provides backwards compatibility with Zipkin
by accepting spans in Zipkin formats (Thrift or JSON v1/v2) over HTTP. Switching from Zipkin backend is just a matter
of routing the traffic from Zipkin libraries to the Jaeger backend.

## Related Repositories

### Documentation

  * Published: http://jaeger.readthedocs.io/en/latest/
  * Source: https://github.com/jaegertracing/documentation

### Instrumentation Libraries

 * [Go client](https://github.com/jaegertracing/jaeger-client-go)
 * [Java client](https://github.com/jaegertracing/jaeger-client-java)
 * [Python client](https://github.com/jaegertracing/jaeger-client-python)
 * [Node.js client](https://github.com/jaegertracing/jaeger-client-node)
 * [C++ client](https://github.com/jaegertracing/cpp-client)

### Deployment

  * [Kubernetes templates](https://github.com/jaegertracing/jaeger-kubernetes)
  * [OpenShift templates](https://github.com/jaegertracing/jaeger-openshift)

### Components

 * [UI](https://github.com/jaegertracing/jaeger-ui)
 * [Data model](https://github.com/jaegertracing/jaeger-idl)
 * [Shared libs](https://github.com/jaegertracing/jaeger-lib)

## Building From Source

See [CONTRIBUTING](./CONTRIBUTING.md).

## Contributing

See [CONTRIBUTING](./CONTRIBUTING.md).

## Project Status Bi-Weekly Meeting

The Jaeger contributors meet bi-weekly, and everyone is welcome to join.
[Agenda and meeting details here](https://docs.google.com/document/d/1ZuBAwTJvQN7xkWVvEFXj5WU9_JmS5TPiNbxCJSvPqX0/).

## Roadmap

See http://jaeger.readthedocs.io/en/latest/roadmap/

## Questions, Discussions, Bug Reports

Reach project contributors via these channels:

 * [jaeger-tracing mail group](https://groups.google.com/forum/#!forum/jaeger-tracing)
 * [Gitter chat room](https://gitter.im/jaegertracing/Lobby)
 * [Github issues](https://github.com/jaegertracing/jaeger/issues)

## Adopters

Jaeger as a product consists of multiple components. We want to support different types of users,
whether they are only using our instrumentation libraries or full end to end Jaeger installation,
whether it runs in production or you use it to troubleshoot issues in development.

Please see [ADOPTERS.md](./ADOPTERS.md) for some of the organizations using Jaeger today.
If you would like to add your organization to the list, please comment on our
[survey issue](https://github.com/jaegertracing/jaeger/issues/207).

## License

[Apache 2.0 License](./LICENSE).

[doc]: https://jaegertracing.netlify.com/docs/
[godoc-img]: https://godoc.org/github.com/jaegertracing/jaeger?status.svg
[godoc]: https://godoc.org/github.com/jaegertracing/jaeger
[ci-img]: https://travis-ci.org/jaegertracing/jaeger.svg?branch=master
[ci]: https://travis-ci.org/jaegertracing/jaeger
[cov-img]: https://coveralls.io/repos/jaegertracing/jaeger/badge.svg?branch=master
[cov]: https://coveralls.io/github/jaegertracing/jaeger?branch=master
[dapper]: https://research.google.com/pubs/pub36356.html
[ubeross]: http://uber.github.io
[ot-badge]: https://img.shields.io/badge/OpenTracing--1.x-inside-blue.svg
[hotrod-tutorial]: https://medium.com/@YuriShkuro/take-opentracing-for-a-hotrod-ride-f6e3141f7941
[gitter]: https://gitter.im/jaegertracing/Lobby
[gitter-img]: http://img.shields.io/badge/gitter-join%20chat%20%E2%86%92-brightgreen.svg

[//]: # (md-to-godoc-ignore)
