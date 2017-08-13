<img align="right" width="290" height="290" src="http://jaeger.readthedocs.io/en/latest/images/jaeger-vector.svg">

[![Build Status][ci-img]][ci] [![Coverage Status][cov-img]][cov] [![ReadTheDocs][doc-img]][doc] [![GoDoc][godoc-img]][godoc] [![Gitter chat][gitter-img]][gitter] [![OpenTracing-1.0][ot-badge]](http://opentracing.io)

# Jaeger - a Distributed Tracing System

Jaeger, inspired by [Dapper][dapper] and [OpenZipkin](http://zipkin.io),
is a distributed tracing system released as open source by [Uber Technologies][ubeross].
It can be used for monitoring microservice-based architectures:

  * Distributed context propagation
  * Distributed transaction monitoring
  * Root cause analysis
  * Service dependency analysis
  * Performance / latency optimization

See also:

  * Jaeger [documentation][doc] for getting started, operational details, and other information.
  * Blog post [Evolving Distributed Tracing at Uber](https://eng.uber.com/distributed-tracing/).
  * Tutorial / walkthrough [Take OpenTracing for a HotROD ride][hotrod-tutorial].

## Related Repositories

### Instrumentation Libraries

 * [Go client](https://github.com/uber/jaeger-client-go)
 * [Java client](https://github.com/uber/jaeger-client-java)
 * [Python client](https://github.com/uber/jaeger-client-python)
 * [Node.js client](https://github.com/uber/jaeger-client-node)

### Contributions

[Jaeger contributions](https://github.com/jaegertracing) organization, including:

  * [Kubernetes templates](https://github.com/jaegertracing/jaeger-kubernetes)
  * [OpenShift templates](https://github.com/jaegertracing/jaeger-openshift)

### Components

 * [UI](https://github.com/uber/jaeger-ui)
 * [Data model](https://github.com/uber/jaeger-idl)

## Contributing

See [CONTRIBUTING](./CONTRIBUTING.md).

## Questions, Discussions, Bug Reports

Reach project contributors via these channels:

 * [jaeger-tracing mail group](https://groups.google.com/forum/#!forum/jaeger-tracing)
 * [Gitter chat room](https://gitter.im/jaegertracing/Lobby)
 * [Github issues](https://github.com/uber/jaeger/issues)

## Adopters

Jaeger is [used in production](./ADOPTERS.md) by organizations such as Atlassian, Orange, Red Hat and Uber.

If you use Jaeger in production, please consider adding yourself to our [ADOPTERS.md](./ADOPTERS.md)

## License

[The MIT License](./LICENSE).

Note: we are planning to switch to Apache License v2.0

[doc-img]: https://readthedocs.org/projects/jaeger/badge/?version=latest
[doc]: http://jaeger.readthedocs.org/en/latest/
[godoc-img]: https://godoc.org/github.com/uber/jaeger?status.svg
[godoc]: https://godoc.org/github.com/uber/jaeger
[ci-img]: https://travis-ci.org/uber/jaeger.svg?branch=master
[ci]: https://travis-ci.org/uber/jaeger
[cov-img]: https://coveralls.io/repos/uber/jaeger/badge.svg?branch=master
[cov]: https://coveralls.io/github/uber/jaeger?branch=master
[dapper]: https://research.google.com/pubs/pub36356.html
[ubeross]: http://uber.github.io
[ot-badge]: https://img.shields.io/badge/OpenTracing--1.x-inside-blue.svg
[hotrod-tutorial]: https://medium.com/@YuriShkuro/take-opentracing-for-a-hotrod-ride-f6e3141f7941
[gitter]: https://gitter.im/jaegertracing/Lobby
[gitter-img]: http://img.shields.io/badge/gitter-join%20chat%20%E2%86%92-brightgreen.svg

[//]: # (md-to-godoc-ignore)
