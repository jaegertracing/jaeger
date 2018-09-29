Changes by Version
==================

1.8.0 (unreleased)
------------------

#### Backend Changes

##### Breaking Changes

- Consolidate query metrics and include result tag ([#1075](https://github.com/jaegertracing/jaeger/pull/1075) and [#1096](https://github.com/jaegertracing/jaeger/pull/1096), [@objectiser](https://github.com/objectiser))
- Make the metrics produced by jaeger query scoped to the query component, and generated for all span readers (not just ES) ([#1074](https://github.com/jaegertracing/jaeger/pull/1074), [@objectiser](https://github.com/objectiser))


1.7.0 (2018-09-19)
------------------

#### UI Changes

- Compare two traces ([#228](https://github.com/jaegertracing/jaeger-ui/pull/228), [@tiffon](https://github.com/tiffon))
- Make tags clickable ([#223](https://github.com/jaegertracing/jaeger-ui/pull/223), [@divdavem](https://github.com/divdavem))
- Directed graph as React component ([#224](https://github.com/jaegertracing/jaeger-ui/pull/224), [@tiffon](https://github.com/tiffon))
- Timeline Expand and Collapse Features ([#221](https://github.com/jaegertracing/jaeger-ui/issues/221), [@davit-y](https://github.com/davit-y))
- Integrate Google Analytics into Search Page ([#220](https://github.com/jaegertracing/jaeger-ui/issues/220), [@davit-y](https://github.com/davit-y))

#### Backend Changes

##### Breaking changes

- `jaeger-standalone` binary has been renamed to `jaeger-all-in-one`. This change also includes package rename from `standalone` to `all-in-one` ([#1062](https://github.com/jaegertracing/jaeger/pull/1062), [@pavolloffay](https://github.com/pavolloffay))

##### New Features

- (Experimental) Allow storing tags as object fields in Elasticsearch for better Kibana support(([#1018](https://github.com/jaegertracing/jaeger/pull/1018), [@pavolloffay](https://github.com/pavolloffay))
- Enable tracing of Cassandra queries ([#1038](https://github.com/jaegertracing/jaeger/pull/1038), [@yurishkuro](https://github.com/yurishkuro))
- Make Elasticsearch index configurable ([#1009](https://github.com/jaegertracing/jaeger/pull/1009), [@pavolloffay](https://github.com/pavoloffay))
- Add flags to allow changing ports for HotROD services ([#951](https://github.com/jaegertracing/jaeger/pull/951), [@cboornaz17](https://github.com/cboornaz17))
- (Experimental) Kafka ingester ([#952](https://github.com/jaegertracing/jaeger/pull/952), [#942](https://github.com/jaegertracing/jaeger/pull/942), [#944](https://github.com/jaegertracing/jaeger/pull/944), [#940](https://github.com/jaegertracing/jaeger/pull/940), [@davit-y](https://github.com/davit-y) and [@vprithvi](https://github.com/vprithvi)))
- Use tags in agent metrics ([#950](https://github.com/jaegertracing/jaeger/pull/950), [@eundoosong](https://github.com/eundoosong))
- Add support for Cassandra reconnect interval ([#934](https://github.com/jaegertracing/jaeger/pull/934), [@nyanshak](https://github.com/nyanshak))

1.6.0 (2018-07-10)
------------------

#### Backend Changes

##### Breaking Changes!!!

- The storage implementations no longer write the parentSpanID field to storage (#856).
  If you are upgrading to this version, **you must upgrade query service first**!

- Update Dockerfiles to reference executable via ENTRYPOINT (#815) by Zachary DiCesare (@zdicesare)

  It is no longer necessary to specify the binary name when passing flags to containers.
  For example, to execute the `help` command of the collector, instead of
  ```
  $ docker run -it --rm jaegertracing/jaeger-collector /go/bin/collector-linux help
  ```
  run
  ```
  $ docker run -it --rm jaegertracing/jaeger-collector help
  ```

- Detect HTTP payload format from Content-Type (#916) by Yuri Shkuro (@yurishkuro)

  When submitting spans in Thrift format to HTTP endpoint `/api/traces`,
  the `format` argument is no longer required, but the Content-Type header
  must be set to "application/vnd.apache.thrift.binary".

- Change metric tag from "service" to "svc" (#883) by Won Jun Jang (@black-adder)

##### New Features

- Add Kafka as a Storage Plugin (#862) by David Yeghshatyan (@davit-y)

  The collectors can be configured to write spans to Kafka for further data mining.

- Package static assets inside the query-service binary (#918) by Yuri Shkuro (@yurishkuro)

  It is no longer necessary (but still possible) to pass the path to UI static assets
  to jaeger-query and jaeger-standalone binaries.

- Replace domain model with Protobuf/gogo-generated model (#856) by Yuri Shkuro (@yurishkuro)

  First step towards switching to Protobuf and gRPC.

- Include HotROD binary in the distributions (#917) by Yuri Shkuro (@yurishkuro)
- Improve HotROD demo (#915) by Yuri Shkuro (@yurishkuro)
- Add DisableAutoDiscovery param to cassandra config (#912) by Bill Westlin (@whistlinwilly)
- Add connCheckTimeout flag to agent (#911) by Henrique Rodrigues (@Henrod)
- Ability to use multiple storage types (#880) by David Yeghshatyan (@davit-y)

##### Minor Improvements

- [ES storage] Log number of total and failed requests (#902) by Tomasz Adamski (@tmszdmsk)
- [ES storage] Do not log requests on error (#901) by Tomasz Adamski (@tmszdmsk)
- [ES storage] Do not exceed ES _id length limit (#905) by Łukasz Harasimowicz (@harnash) and Tomasz Adamski (@tmszdmsk)
- Add cassandra index filter (#876) by Won Jun Jang (@black-adder)
- Close span writer in standalone (#863) (4 weeks ago) by Pavol Loffay (@pavolloffay)
- Log configuration options for memory storage (#852) (6 weeks ago) by Juraci Paixão Kröhling (@jpkrohling)
- Update collector metric counters to have a name (#886) by Won Jun Jang (@black-adder)
- Add CONTRIBUTING_GUIDELINES.md (#864) by (@PikBot)

1.5.0 (2018-05-28)
------------------

#### Backend Changes

- Add bounds to memory storage (#845) by Juraci Paixão Kröhling (@jpkrohling)
- Add metric for debug traces (#796) by Won Jun Jang (@black-adder)
- Change metrics naming scheme (#776) by Juraci Paixão Kröhling (@jpkrohling)
- Remove ParentSpanID from domain model (#831) by Yuri Shkuro (@yurishkuro)
- Add ability to adjust static sampling probabilities per operation (#827) by Won Jun Jang (@black-adder)
- Support log-level flag on agent (#828) by Won Jun Jang (@black-adder)
- Add healthcheck to standalone (#784) by Eundoo Song (@eundoosong)
- Do not use KeyValue fields directly and use KeyValues as decorator only (#810) by Yuri Shkuro (@yurishkuro)
- Upgrade to go 1.10 (#792) by Prithvi Raj (@vprithvi)
- Do not create Cassandra index if it already exists (#782) by Greg Swift (@gregswift)

#### UI Changes

- None

1.4.1 (2018-04-21)
------------------

#### Backend Changes

- Publish binaries for Linux, Darwin, and Windows (#765) - thanks to @grounded042

#### UI Changes

##### New Features

- View Trace JSON buttons return formatted JSON (fixes [#199](https://github.com/jaegertracing/jaeger-ui/issues/199))


1.4.0 (2018-04-20)
------------------

#### Backend Changes

##### New Features

- Support traces with >10k spans in Elasticsearch (#668) - thanks to @sramakr

##### Fixes

- Allow slash '/' in service names (#586)
- Log errors from HotROD services (#769)


1.3.0 (2018-03-26)
------------------

#### Backend Changes

##### New Features

- Add sampling handler with file-based configuration for agents to query (#720) (#674) <Won Jun Jang>
- Allow overriding base path for UI/API routes and remove --query.prefix (#748) <Yuri Shkuro>
- Add Dockerfile for hotrod example app (#694) <Guilherme Baufaker Rêgo>
- Publish hotrod image to docker hub (#702) <Pavol Loffay>
- Dockerize es-index-cleaner script (#741) <Pavol Loffay>
- Add a flag to control Cassandra consistency level (#700) <Yuri Shkuro>
- Collect metrics from ES bulk service (#688) <Pavol Loffay>
- Allow zero replicas for Elasticsearch (#754) <bharat-p>

##### Fixes

- Apply namespace when creating Prometheus metrics factory (fix for #732) (#733) <Yuri Shkuro>
- Disable double compression on Prom Handler - fixes #697 (#735) <Juraci Paixão Kröhling>
- Use the default metricsFactory if not provided (#739) <Louis-Etienne>
- Avoid duplicate expvar metrics - fixes #716 (#726) <Yuri Shkuro>
- Make sure different tracers in HotROD process use different random generator seeds (#718) <Yuri Shkuro>
- Test that processes with identical tags are deduped (#708) <Yuri Shkuro>
- When converting microseconds to time.Time ensure UTC timezone (#712) <Prithvi Raj>
- Add to WaitGroup before the goroutine creation (#711) <Cruth kvinc>
- Pin testify version to ^1.2.1 (#710) <Pavol Loffay>

#### UI Changes

##### New Features

- Support running Jaeger behind a reverse proxy (fixes [#42](https://github.com/jaegertracing/jaeger-ui/issues/42))
- Track Javascript errors via Google Analytics (fixes [#39](https://github.com/jaegertracing/jaeger-ui/issues/39))
- Add Google Analytics event tracking for actions in trace view ([#191](https://github.com/jaegertracing/jaeger-ui/issues/191))

##### Fixes

- Clearly identify traces without a root span (fixes [#190](https://github.com/jaegertracing/jaeger-ui/issues/190))
- Fix [#166](https://github.com/jaegertracing/jaeger-ui/issues/166) JS error on search page after viewing 404 trace

#### Documentation Changes


1.2.0 (2018-02-07)
------------------

#### Backend Changes

##### New Features

- Use elasticsearch bulk API (#656) <Pavol Loffay>
- Support archive storage in the query-service (#604) <Yuri Shkuro>
- Introduce storage factory framework and composable CLI (#625) <Yuri Shkuro>
- Make agent host port configurable in hotrod (#663) <Pavol Loffay>
- Add signal handling to standalone (#657) <Pavol Loffay>

##### Fixes

- Remove the override of GOMAXPROCS (#679) <Cruth kvinc>
- Use UTC timezone for ES indices (#646) <Pavol Loffay>
- Fix elasticsearch create index race condition error (#641) <Pavol Loffay>

#### UI Changes

##### New Features

- Use Ant Design instead of Semantic UI (https://github.com/jaegertracing/jaeger-ui/pull/169)
  - Fix [#164](https://github.com/jaegertracing/jaeger-ui/issues/164) - Use Ant Design instead of Semantic UI
  - Fix [#165](https://github.com/jaegertracing/jaeger-ui/issues/165) - Search results are shown without a date
  - Fix [#69](https://github.com/jaegertracing/jaeger-ui/issues/69) - Missing endpoints in jaeger ui dropdown

##### Fixes

- Fix 2 digit lookback (12h, 24h) parsing (https://github.com/jaegertracing/jaeger-ui/issues/167)


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

