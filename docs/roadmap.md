# Roadmap

The following is only a selection of some of the major features we plan to implement in the near future (6-12 months).
To get a more complete overview of planned features and current work, see the issue trackers for the various repositories,
for example, the [Jaeger backend](https://github.com/jaegertracing/jaeger/issues/).

## Adaptive Sampling

The most common way of using Jaeger client libraries is with probabilistic sampling which makes a determination
if a new trace should be sampled or not. Sampling is necessary to control the amount of tracing data reaching
the storage backend. There are two issues with the current approach:

  1. Individual microservices have little insight into what the appropriate sampling rate should be.
     For example, 0.001 probability (one trace per second per service instance) might seem reasonable,
     but if the fanout in some downstream services is very high it might flood the tracing backend.
  1. Sampling rates are defined on a per-service basis. If a service has two endpoints with vastly different
     throughputs, then its sampling rate will be driven on the high QPS endpoint, which may leave the low QPS
     endpoint never sampled. For example, if the QPS of the endpoints is different by a factor of 100, and the
     probability is set to 0.001, then the low QPS traffic will have only 1 in 100,000 chance to be sampled.

See issue tracker for more info: [jaeger/issues/365](https://github.com/jaegertracing/jaeger/issues/365).

## Support for Large Traces in the UI

[jaeger-ui/milestone/1](https://github.com/jaegertracing/jaeger-ui/milestone/1)

## Instrumentation Libraries in More Languages

[jaeger/issues/366](https://github.com/jaegertracing/jaeger/issues/366)

## Data Pipeline

Post-collection data pipeline for trace aggregation and data mining based on Apache Flink.

## Drop-in Replacement for Zipkin

Features for Jaeger backend to be a drop-in replacement for Zipkin backend.

[jaeger/milestone/2](https://github.com/jaegertracing/jaeger/milestone/2)

## Path-Based Dependency Diagrams

Service dependency diagram currently available in Jaeger (as of v0.7.0) only shows service-to-service links.
Such diagrams are of limited usefulness because they do not account for the actual
execution paths passing through the service, where requests to one endpoint may
involve one set of downstream dependencies which are different from dependencies
of another endpoint. We plan to open source the aggregation module that builds
path-based dependency diagrams with the following features:

  * Show all upstream and downstream dependencies of a selected service `postmaster`,
    not just the immediate neighbors;
  * Can be shown at the service level or at the endpoint level;
  * Interactive, for example using `cli_user2` as a filter grays out the paths in the graph
    that are not relevant to requests passing through both `cli_user2` and `postmaster`.

<img src="../images/path-dependency.svg">

## Latency Histograms

Jaeger traces contain a wealth of information about how the system executes a given request.
But how can one find interesting traces? Latency histograms allow not only navigation to the
interesting traces, such as those representing the long tail, but also analysis of the
request paths from upstream services. In the screenshot below we see how selecting
a portion of the histogram reveals the breakdowns of the endpoints and upstream callers
that are responsible for the long tail.

<img src="../images/latency-histrogram.png">

## Trace Quality Metrics

When deploying a distributed tracing solution like Jaeger in large organizations
that utilize many different technologies and programming languages,
there are always questions about how much of the architecture is integrated
with tracing, what is the quality of the instrumentation, are there microservices
that are using stale versions of instrumentation libraries, etc.

Trace Quality engine ([jaeger/issues/367](https://github.com/jaegertracing/jaeger/issues/367))
runs analysis on all traces collected in the backend, inspects them for known completeness
and quality problems, and provides summary reports to service owners with suggestions on
improving the quality metrics and links to sample traces that exhibit the issues.

## Dynamic Configuration

We need a dynamic configuration solution ([jaeger/issues/355](https://github.com/jaegertracing/jaeger/issues/355))
that comes in handy in various scenarios:

  * Blacklisting services,
  * Overriding sampling probabilities,
  * Controlling server-side downsampling rate,
  * Black/whitelisting services for adaptive sampling,
  * etc.

## Long Term Roadmap

* Multi-Tenancy ([mailgroup thread](https://groups.google.com/forum/#!topic/jaeger-tracing/PcxftflO4_o))
* Cloud and Multi-DC strategy
* Post-Trace Sampling

