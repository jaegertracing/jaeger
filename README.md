<img align="right" width="290" height="290" src="https://www.jaegertracing.io/img/jaeger-vector.svg">


[![CII Best Practices](https://bestpractices.coreinfrastructure.org/projects/1273/badge)](https://bestpractices.coreinfrastructure.org/projects/1273)
[![Mentioned in Awesome Go](https://awesome.re/mentioned-badge-flat.svg)](https://github.com/avelino/awesome-go#performance)
[![OpenTracing-1.0][ot-badge]](https://opentracing.io)

[![Gitter chat][gitter-img]][gitter] [![Project+Community stats][community-badge]][community-stats]

[![Unit Tests][ci-img]][ci] [![Coverage Status][cov-img]][cov] [![FOSSA Status][fossa-img]][ci]

# Jaeger - a Distributed Tracing System

Jaeger, inspired by [Dapper][dapper] and [OpenZipkin](https://zipkin.io),
is a distributed tracing platform created by [Uber Technologies][ubeross]
and donated to [Cloud Native Computing Foundation](https://cncf.io).
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

Jaeger is hosted by the [Cloud Native Computing Foundation](https://cncf.io) (CNCF) as the 7th top-level project (graduated in October 2019). If you are a company that wants to help shape the evolution of technologies that are container-packaged, dynamically-scheduled and microservices-oriented, consider joining the CNCF. For details about who's involved and how Jaeger plays a role, read the CNCF [Jaeger incubation announcement](https://www.cncf.io/blog/2017/09/13/cncf-hosts-jaeger/) and [Jaeger graduation announcement](https://www.cncf.io/announcement/2019/10/31/cloud-native-computing-foundation-announces-jaeger-graduation/).

## Get Involved

Jaeger is an open source project with open governance. We welcome contributions from the community, and we’d love your help to improve and extend the project. Here are [some ideas](https://www.jaegertracing.io/get-involved/) for how to get involved. Many of them don’t even require any coding.

## Features

### High Scalability

Jaeger backend is designed to have no single points of failure and to scale with the business needs.
For example, any given Jaeger installation at Uber is typically processing several billions of spans per day.

### Native support for OpenTracing

Jaeger backend, Web UI, and instrumentation libraries have been designed from the ground up to support the [OpenTracing standard](https://opentracing.io/specification/).
  * Represent traces as directed acyclic graphs (not just trees) via [span references](https://github.com/opentracing/specification/blob/master/specification.md#references-between-spans)
  * Support strongly typed span _tags_ and _structured logs_
  * Support general distributed context propagation mechanism via _baggage_

#### OpenTelemetry

On 28-May-2019, [the OpenTracing and OpenCensus projects announced](https://medium.com/opentracing/merging-opentracing-and-opencensus-f0fe9c7ca6f0) their intention to merge into a new CNCF project called [OpenTelemetry](https://opentelemetry.io). The Jaeger and OpenTelemetry projects have different goals. OpenTelemetry aims to provide APIs and SDKs in multiple languages to allow applications to export various telemetry data out of the process, to any number of metrics and tracing backends. The Jaeger project is primarily the tracing backend that receives tracing telemetry data and provides processing, aggregation, data mining, and visualizations of that data. The Jaeger client libraries do overlap with OpenTelemetry in functionality. OpenTelemetry will natively support Jaeger as a tracing backend and eventually might make Jaeger native clients unnecessary. For more information please refer to a blog post [Jaeger and OpenTelemetry](https://medium.com/jaegertracing/jaeger-and-opentelemetry-1846f701d9f2).

### Multiple storage backends

Jaeger supports two popular open source NoSQL databases as trace storage backends: Cassandra and Elasticsearch.
There is also embedded database support using [Badger](https://github.com/dgraph-io/badger).
There are ongoing community experiments using other databases, such as ScyllaDB, InfluxDB, Amazon DynamoDB.
Jaeger also ships with a simple in-memory storage for testing setups.

### Modern Web UI

Jaeger Web UI is implemented in Javascript using popular open source frameworks like React. Several performance
improvements have been released in v1.0 to allow the UI to efficiently deal with large volumes of data and to display
traces with tens of thousands of spans (e.g. we tried a trace with 80,000 spans).

### Cloud Native Deployment

Jaeger backend is distributed as a collection of Docker images. The binaries support various configuration methods,
including command line options, environment variables, and configuration files in multiple formats (yaml, toml, etc.)
Deployment to Kubernetes clusters is assisted by [Kubernetes templates](https://github.com/jaegertracing/jaeger-kubernetes)
and a [Helm chart](https://github.com/kubernetes/charts/tree/master/incubator/jaeger).

### Observability

All Jaeger backend components expose [Prometheus](https://prometheus.io/) metrics by default (other metrics backends are
also supported). Logs are written to standard out using the structured logging library [zap](https://github.com/uber-go/zap).

### Security

Third-party security audits of Jaeger are available in https://github.com/jaegertracing/security-audits. Please see [Issue #1718](https://github.com/jaegertracing/jaeger/issues/1718) for the summary of available security mechanisms in Jaeger.

### Backwards compatibility with Zipkin

Although we recommend instrumenting applications with OpenTracing API and binding to Jaeger client libraries to benefit
from advanced features not available elsewhere, if your organization has already invested in the instrumentation
using Zipkin libraries, you do not have to rewrite all that code. Jaeger provides backwards compatibility with Zipkin
by accepting spans in Zipkin formats (Thrift or JSON v1/v2) over HTTP. Switching from Zipkin backend is just a matter
of routing the traffic from Zipkin libraries to the Jaeger backend.

## Related Repositories

### Documentation

  * Published: https://www.jaegertracing.io/docs/
  * Source: https://github.com/jaegertracing/documentation

### Instrumentation Libraries

 * [Go client](https://github.com/jaegertracing/jaeger-client-go)
 * [Java client](https://github.com/jaegertracing/jaeger-client-java)
 * [Python client](https://github.com/jaegertracing/jaeger-client-python)
 * [Node.js client](https://github.com/jaegertracing/jaeger-client-node)
 * [C++ client](https://github.com/jaegertracing/jaeger-client-cpp)
 * [C# client](https://github.com/jaegertracing/jaeger-client-csharp)

### Deployment

  * [Jaeger Operator for Kubernetes](https://github.com/jaegertracing/jaeger-operator#getting-started)

### Components

 * [UI](https://github.com/jaegertracing/jaeger-ui)
 * [Data model](https://github.com/jaegertracing/jaeger-idl)
 * [Shared libs](https://github.com/jaegertracing/jaeger-lib)

## Building From Source

See [CONTRIBUTING](./CONTRIBUTING.md).

## Contributing

See [CONTRIBUTING](./CONTRIBUTING.md).

## Maintainers

Below are the official maintainers of the Jaeger project. Please use `@jaegertracing/jaeger-maintainers` to tag them on issues / PRs.

* [@black-adder](https://github.com/black-adder)
* [@joe-elliott](https://github.com/joe-elliott)
* [@jpkrohling](https://github.com/jpkrohling)
* [@objectiser](https://github.com/objectiser)
* [@pavolloffay](https://github.com/pavolloffay)
* [@tiffon](https://github.com/tiffon)
* [@vprithvi](https://github.com/vprithvi)
* [@yurishkuro](https://github.com/yurishkuro)

Some repositories under [jaegertracing](https://github.com/jaegertracing) org have additional maintainers.

## Project Status Bi-Weekly Meeting

The Jaeger contributors meet bi-weekly, and everyone is welcome to join.
[Agenda and meeting details here](https://docs.google.com/document/d/1ZuBAwTJvQN7xkWVvEFXj5WU9_JmS5TPiNbxCJSvPqX0/).

## Roadmap

See https://www.jaegertracing.io/docs/roadmap/

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

[doc]: https://jaegertracing.io/docs/
[godoc-img]: https://godoc.org/github.com/jaegertracing/jaeger?status.svg
[godoc]: https://godoc.org/github.com/jaegertracing/jaeger
[ci-img]: https://github.com/jaegertracing/jaeger/workflows/Unit%20Tests/badge.svg?branch=master
[ci]: https://github.com/jaegertracing/jaeger/actions?query=branch%3Amaster
[cov-img]: https://codecov.io/gh/jaegertracing/jaeger/branch/master/graph/badge.svg
[cov]: https://codecov.io/gh/jaegertracing/jaeger/branch/master/
[fossa-img]: https://github.com/jaegertracing/jaeger/workflows/FOSSA/badge.svg?branch=master
[dapper]: https://research.google.com/pubs/pub36356.html
[ubeross]: https://uber.github.io
[ot-badge]: https://img.shields.io/badge/OpenTracing--1.x-inside-blue.svg
[community-badge]: https://img.shields.io/badge/Project+Community-stats-blue.svg
[community-stats]: https://all.devstats.cncf.io/d/54/project-health?orgId=1&var-repogroup_name=Jaeger
[hotrod-tutorial]: https://medium.com/@YuriShkuro/take-opentracing-for-a-hotrod-ride-f6e3141f7941
[gitter]: https://gitter.im/jaegertracing/Lobby
[gitter-img]: https://img.shields.io/badge/gitter-join%20chat%20%E2%86%92-brightgreen.svg
