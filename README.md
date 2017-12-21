<img align="right" width="290" height="290" src="http://jaeger.readthedocs.io/en/latest/images/jaeger-vector.svg">

[![Build Status][ci-img]][ci] [![Coverage Status][cov-img]][cov] [![ReadTheDocs][doc-img]][doc] [![GoDoc][godoc-img]][godoc] [![Gitter chat][gitter-img]][gitter] [![OpenTracing-1.0][ot-badge]](http://opentracing.io) [![FOSSA Status](https://app.fossa.io/api/projects/git%2Bgithub.com%2Fjaegertracing%2Fjaeger.svg?type=shield)](https://app.fossa.io/projects/git%2Bgithub.com%2Fjaegertracing%2Fjaeger?ref=badge_shield) [![CII Best Practices](https://bestpractices.coreinfrastructure.org/projects/1273/badge)](https://bestpractices.coreinfrastructure.org/projects/1273)

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

<img src="https://www.cncf.io/wp-content/uploads/2016/09/logo_cncf.png">

Jaeger is hosted by the [Cloud Native Computing Foundation](https://cncf.io) (CNCF). If you are a company that wants to help shape the evolution of technologies that are container-packaged, dynamically-scheduled and microservices-oriented, consider joining the CNCF. For details about who's involved and how Jaeger plays a role, read the CNCF [announcement](https://www.cncf.io/blog/2017/09/13/cncf-hosts-jaeger/).

## Related Repositories

### Instrumentation Libraries

 * [Go client](https://github.com/jaegertracing/jaeger-client-go)
 * [Java client](https://github.com/jaegertracing/jaeger-client-java)
 * [Python client](https://github.com/jaegertracing/jaeger-client-python)
 * [Node.js client](https://github.com/jaegertracing/jaeger-client-node)
 * [C++ client](https://github.com/jaegertracing/cpp-client)

### Contributions

[Jaeger contributions](https://github.com/jaegertracing) organization, including:

  * [Kubernetes templates](https://github.com/jaegertracing/jaeger-kubernetes)
  * [OpenShift templates](https://github.com/jaegertracing/jaeger-openshift)

### Components

 * [UI](https://github.com/jaegertracing/jaeger-ui)
 * [Data model](https://github.com/jaegertracing/jaeger-idl)

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

[doc-img]: https://readthedocs.org/projects/jaeger/badge/?version=latest
[doc]: http://jaeger.readthedocs.org/en/latest/
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

