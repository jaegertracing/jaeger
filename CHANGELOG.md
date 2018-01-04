Changes by Version
==================

1.1.0 (2018-01-03)
------------------

#### Backend Changes

##### New Features

- Add support for retrieving unadjusted/raw traces (#615)
- Add CA certificates to collector/query images (#485)
- Parse zipkin v2 high trace id (#596)

##### Fixes

- Skip nil and zero length hits in ElasticSearch storage (#601)
- Make Cassandra service_name_index inserts idempotent (#587)
- Align atomic int64 to word boundary to fix SIGSEGV (#592)
- Add adjuster that removes bad span references (#614)
- Set operationNames cache initial capacity to 10000 (#621)

#### UI Changes

##### New Features

- Change tag search input syntax to logfmt (https://github.com/jaegertracing/jaeger-ui/issues/145)
- Make threshold for enabling DAG view configurable (https://github.com/jaegertracing/jaeger-ui/issues/130)
- Show better error messages for failed API calls (https://github.com/jaegertracing/jaeger-ui/issues/127)
- Add View Option for raw/unadjusted trace (https://github.com/jaegertracing/jaeger-ui/issues/153)
- Add timezone tooltip to custom lookback form-field (https://github.com/jaegertracing/jaeger-ui/pull/161)

##### Fixes

- Use consistent icons for logs expanded/collapsed (https://github.com/jaegertracing/jaeger-ui/issues/86)
- Encode service name in API calls to allow '/' (https://github.com/jaegertracing/jaeger-ui/issues/138)
- Fix endless trace HTTP requests (https://github.com/jaegertracing/jaeger-ui/issues/128)
- Fix JSON view when running in dev mode (https://github.com/jaegertracing/jaeger-ui/issues/139)
- Fix trace name resolution (https://github.com/jaegertracing/jaeger-ui/pull/134)
- Only JSON.parse JSON strings in tags/logs values (https://github.com/jaegertracing/jaeger-ui/pull/162)


1.0.0 (2017-12-04)
------------------

#### Backend Changes

- Support Prometheus metrics as default for all components (#516)
- Enable TLS client connections to Cassandra (#555)
- Fix issue where Domain to UI model converter double reports references (#579)

#### UI Changes

- Make dependencies tab configurable (#122)


0.10.0 (2017-11-17)
------------------

#### UI Changes

- Verify stored search settings [jaegertracing/jaeger-ui#111](https://github.com/jaegertracing/jaeger-ui/pull/111)
- Fix browser back button not working correctly [jaegertracing/jaeger-ui#110](https://github.com/jaegertracing/jaeger-ui/pull/110)
- Handle FOLLOWS_FROM ref type [jaegertracing/jaeger-ui#118](https://github.com/jaegertracing/jaeger-ui/pull/118)

#### Backend Changes

- Allow embedding custom UI config in index.html [#490](https://github.com/jaegertracing/jaeger/pull/490)
- Add startTimeMillis field to JSON Spans submitted to ElasticSearch [#491](https://github.com/jaegertracing/jaeger/pull/491)
- Introduce version command and handler [#517](https://github.com/jaegertracing/jaeger/pull/517)
- Fix ElasticSearch aggregation errors when index is empty [#535](https://github.com/jaegertracing/jaeger/pull/535)
- Change package from uber to jaegertracing [#528](https://github.com/jaegertracing/jaeger/pull/528)
- Introduce logging level configuration [#514](https://github.com/jaegertracing/jaeger/pull/514)
- Support Zipkin v2 json [#518](https://github.com/jaegertracing/jaeger/pull/518)
- Add HTTP compression handler [#545](https://github.com/jaegertracing/jaeger/pull/545)


0.9.0 (2017-10-25)
------------------

#### UI Changes

- Refactor trace detail [jaegertracing/jaeger-ui#53](https://github.com/jaegertracing/jaeger-ui/pull/53)
- Virtualized scrolling for trace detail view [jaegertracing/jaeger-ui#68](https://github.com/jaegertracing/jaeger-ui/pull/68)
- Mouseover expands truncated text to full length in left column in trace view [jaegertracing/jaeger-ui#71](https://github.com/jaegertracing/jaeger-ui/pull/71)
- Make left column adjustable in trace detail view [jaegertracing/jaeger-ui#74](https://github.com/jaegertracing/jaeger-ui/pull/74)
- Fix trace mini-map blurriness when < 60 spans [jaegertracing/jaeger-ui#77](https://github.com/jaegertracing/jaeger-ui/pull/77)
- Fix Google Analytics tracking [jaegertracing/jaeger-ui#81](https://github.com/jaegertracing/jaeger-ui/pull/81)
- Improve search dropdowns [jaegertracing/jaeger-ui#84](https://github.com/jaegertracing/jaeger-ui/pull/84)
- Add keyboard shortcuts and minimap UX [jaegertracing/jaeger-ui#93](https://github.com/jaegertracing/jaeger-ui/pull/93)

#### Backend Changes

- Add tracing to the query server [#454](https://github.com/uber/jaeger/pull/454)
- Support configuration files [#462](https://github.com/uber/jaeger/pull/462)
- Add cassandra tag filter [#442](https://github.com/uber/jaeger/pull/442)
- Handle ports > 32k in Zipkin JSON [#488](https://github.com/uber/jaeger/pull/488)


0.8.0 (2017-09-24)
------------------

- Convert to Apache 2.0 License


0.7.0 (2017-08-22)
------------------

- Add health check server to collector and query [#280](https://github.com/uber/jaeger/pull/280)
- Add/fix sanitizer for Zipkin span start time and duration [#333](https://github.com/uber/jaeger/pull/333)
- Support Zipkin json encoding for /api/v1/spans HTTP endpoint [#348](https://github.com/uber/jaeger/pull/348)
- Support Zipkin 128bit traceId and ipv6 [#349](https://github.com/uber/jaeger/pull/349)


0.6.0 (2017-08-09)
------------------

- Add viper/cobra configuration support [#245](https://github.com/uber/jaeger/pull/245) [#307](https://github.com/uber/jaeger/pull/307)
- Add Zipkin /api/v1/spans endpoint [#282](https://github.com/uber/jaeger/pull/282)
- Add basic authenticator to configs for cassandra [#323](https://github.com/uber/jaeger/pull/323)
- Add Elasticsearch storage support


0.5.2 (2017-07-20)
------------------

- Use official Cassandra 3.11 base image [#278](https://github.com/uber/jaeger/pull/278)
- Support configurable metrics backend in the agent [#275](https://github.com/uber/jaeger/pull/275)


0.5.1 (2017-07-03)
------------------

- Bug fix: Query startup should fail when -query.static-files has no trailing slash [#144](https://github.com/uber/jaeger/issues/144)
- Push to Docker Hub on release tags [#246](https://github.com/uber/jaeger/pull/246)


0.5.0 (2017-07-01)
------------------

First numbered release.

