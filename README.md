<img align="right" width="290" height="210" src="http://jaeger.readthedocs.io/en/latest/images/jaeger_vector.svg">

[![ReadTheDocs][doc-img]][doc] [![GoDoc][godoc-img]][godoc] [![Build Status][ci-img]][ci] [![Coverage Status][cov-img]][cov]

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

## Status

Most of the code here is used in production at Uber, but the open source version is currently in **Public Beta** until the first official release.

## Contributing

See [CONTRIBUTING](./CONTRIBUTING.md).

## License

[The MIT License](./LICENSE).

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
[//]: # (md-to-godoc-ignore)
