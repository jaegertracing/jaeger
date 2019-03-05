Changes by Version
==================

1.12.0 (unreleased)
------------------

#### Backend Changes

##### Breaking Changes

##### New Features

##### Bug fixes, Minor Improvements

#### UI Changes


1.11.0 (2019-03-07)
------------------

#### Backend Changes

##### Breaking Changes
- Introduce `kafka.producer` and `kafka.consumer` flags to replace `kafka` flags ([1360](https://github.com/jaegertracing/jaeger/pull/1360), [@ledor473](https://github.com/ledor473))

    The following flags have been deprecated in the Collector and the Ingester:
    ```
    --kafka.brokers
    --kafka.encoding
    --kafka.topic
    ``` 
    
    In the Collector, they are replaced by:
    ```
    --kafka.producer.brokers
    --kafka.producer.encoding
    --kafka.producer.topic
    ```
    
    In the Ingester, they are replaced by:
    ```
    --kafka.consumer.brokers
    --kafka.consumer.encoding
    --kafka.consumer.group-id
    ```
##### New Features

- Support secure gRPC channel between agent and collector ([#1391](https://github.com/jaegertracing/jaeger/pull/1391), [@ghouscht](https://github.com/ghouscht), [@yurishkuro](https://github.com/yurishkuro))

##### Bug fixes, Minor Improvements

- Allow thrift reporter even if grpc hosts are not provided ([#1400](https://github.com/jaegertracing/jaeger/pull/1400), [@pavolloffay](https://github.com/pavolloffay))
- Allow to use TLS with ES basic auth ([#1388](https://github.com/jaegertracing/jaeger/pull/1388), [@pavolloffay](https://github.com/pavolloffay))
- Deprecate colon in index prefix in ES dependency store ([#1386](https://github.com/jaegertracing/jaeger/pull/1386), [@pavolloffay](https://github.com/pavolloffay))
- Make grpc reporter default and add retry ([#1384](https://github.com/jaegertracing/jaeger/pull/1384), [@pavolloffay](https://github.com/pavolloffay))
- Use `CQLSH_HOST` in final call to `cqlsh` ([#1372](https://github.com/jaegertracing/jaeger/pull/1372), [@funny-falcon](https://github.com/funny-falcon))

#### UI Changes

* UI pinned to version 1.1.0. The changelog is available here [v1.1.0](https://github.com/jaegertracing/jaeger-ui/blob/master/CHANGELOG.md#v110-march-3-2019)


1.10.1 (2019-02-21)
------------------

#### Backend Changes

- Discover dependencies table version automatically ([#1364](https://github.com/jaegertracing/jaeger/pull/1364), [@black-adder](https://github.com/black-adder))

##### Breaking Changes

##### New Features

##### Bug fixes, Minor Improvements

- Separate query-service functionality from http handler ([#1312](https://github.com/jaegertracing/jaeger/pull/1312), [@annanay25](https://github.com/annanay25))

#### UI Changes


1.10.0 (2019-02-15)
------------------

#### Backend Changes

##### Breaking Changes

- Remove cassandra SASI indices ([#1328](https://github.com/jaegertracing/jaeger/pull/1328), [@black-adder](https://github.com/black-adder))

Migration Path:

1. Run `plugin/storage/cassandra/schema/migration/v001tov002part1.sh` which will copy dependencies into a csv, update the `dependency UDT`, create a new `dependencies_v2` table, and write dependencies from the csv into the `dependencies_v2` table.
2. Run the collector and query services with the cassandra flag `cassandra.enable-dependencies-v2=true` which will instruct jaeger to write and read to and from the new `dependencies_v2` table.
3. Update [spark job](https://github.com/jaegertracing/spark-dependencies) to write to the new `dependencies_v2` table. The feature will be done in [#58](https://github.com/jaegertracing/spark-dependencies/issues/58).
4. Run `plugin/storage/cassandra/schema/migration/v001tov002part2.sh` which will DELETE the old dependency table and the SASI index.

Users who wish to continue to use the v1 table don't have to do anything as the cassandra flag `cassandra.enable-dependencies-v2` will default to false. Users may migrate on their own timeline however new features will be built solely on the `dependencies_v2` table. In the future, we will remove support for v1 completely.

- Remove `ErrorBusy`, it essentially duplicates `SpansDropped` ([#1091](https://github.com/jaegertracing/jaeger/pull/1091), [@cstyan](https://github.com/cstyan))

##### New Features

- Support certificates in elasticsearch scripts ([#1339](https://github.com/jaegertracing/jaeger/pull/1399), [@pavolloffay](https://github.com/pavolloffay))
- Add ES Rollover support to main indices ([#1309](https://github.com/jaegertracing/jaeger/pull/1309), [@pavolloffay](https://github.com/pavolloffay))
- Load ES auth token from file ([#1319](https://github.com/jaegertracing/jaeger/pull/1319), [@pavolloffay](https://github.com/pavolloffay))
- Add username/password authentication to ES index cleaner ([#1318](https://github.com/jaegertracing/jaeger/pull/1318), [@gregoryfranklin](https://github.com/gregoryfranklin))
- Add implementation of FindTraceIDs function for Elasticsearch reader ([#1280](https://github.com/jaegertracing/jaeger/pull/1280), [@vlamug](https://github.com/vlamug))
- Support archive traces for ES storage ([#1197](https://github.com/jaegertracing/jaeger/pull/1197), [@pavolloffay](https://github.com/pavolloffay))

##### Bug fixes, Minor Improvements

- Use Zipkin annotations if the timestamp is zero ([#1341](https://github.com/jaegertracing/jaeger/pull/1341), [@geobeau](https://github.com/geobeau))
- Use GRPC round robin balancing even if only one hostname ([#1329](https://github.com/jaegertracing/jaeger/pull/1329), [@benley](https://github.com/benley))
- Tolerate whitespaces in ES servers and kafka brokers ([#1305](https://github.com/jaegertracing/jaeger/pull/1305), [@verma-varsha](https://github.com/verma-varsha))
- Let cassandra servers contain whitespace in config ([#1301](https://github.com/jaegertracing/jaeger/pull/1301), [@karlpokus](https://github.com/karlpokus))

#### UI Changes


1.9.0 (2019-01-21)
------------------

#### Backend Changes

##### Breaking Changes

- Change Elasticsearch index prefix from `:` to `-` ([#1284](https://github.com/jaegertracing/jaeger/pull/1284), [@pavolloffay](https://github.com/pavolloffay))

Changed index prefix separator from `:`  to `-` because Elasticsearch 7 does not allow `:` in index name.
Jaeger query still reads from old indices containing `-` as a separator, therefore no configuration or migration changes are required.



- Add CLI configurable `es.max-num-spans` while retrieving spans from ES ([#1283](https://github.com/jaegertracing/jaeger/pull/1283), [@annanay25](https://github.com/annanay25))

The default value is set to 10000. Before no limit was applied.


- Update to jaeger-lib 2 and latest sha for jaeger-client-go, to pick up refactored metric names ([#1282](https://github.com/jaegertracing/jaeger/pull/1282), [@objectiser](https://github.com/objectiser))

Update to latest version of `jaeger-lib`, which includes a change to the naming of counters exported to
prometheus, to follow the convention of using a `_total` suffix, e.g. `jaeger_query_requests` is now
`jaeger_query_requests_total`.

Jaeger go client metrics, previously under the namespace `jaeger_client_jaeger_` are now under
`jaeger_tracer_`.


- Add gRPC metrics to agent ([#1180](https://github.com/jaegertracing/jaeger/pull/1180), [@pavolloffay](https://github.com/pavolloffay))

The following metrics:
```
jaeger_agent_tchannel_reporter_batch_size{format="jaeger"} 0
jaeger_agent_tchannel_reporter_batch_size{format="zipkin"} 0
jaeger_agent_tchannel_reporter_batches_failures{format="jaeger"} 0
jaeger_agent_tchannel_reporter_batches_failures{format="zipkin"} 0
jaeger_agent_tchannel_reporter_batches_submitted{format="jaeger"} 0
jaeger_agent_tchannel_reporter_batches_submitted{format="zipkin"} 0
jaeger_agent_tchannel_reporter_spans_failures{format="jaeger"} 0
jaeger_agent_tchannel_reporter_spans_failures{format="zipkin"} 0
jaeger_agent_tchannel_reporter_spans_submitted{format="jaeger"} 0
jaeger_agent_tchannel_reporter_spans_submitted{format="zipkin"} 0

jaeger_agent_collector_proxy{endpoint="baggage",result="err"} 0
jaeger_agent_collector_proxy{endpoint="baggage",result="ok"} 0
jaeger_agent_collector_proxy{endpoint="sampling",result="err"} 0
jaeger_agent_collector_proxy{endpoint="sampling",result="ok"} 0
```
have been renamed to:
```
jaeger_agent_reporter_batch_size{format="jaeger",protocol="tchannel"} 0
jaeger_agent_reporter_batch_size{format="zipkin",protocol="tchannel"} 0
jaeger_agent_reporter_batches_failures{format="jaeger",protocol="tchannel"} 0
jaeger_agent_reporter_batches_failures{format="zipkin",protocol="tchannel"} 0
jaeger_agent_reporter_batches_submitted{format="jaeger",protocol="tchannel"} 0
jaeger_agent_reporter_batches_submitted{format="zipkin",protocol="tchannel"} 0
jaeger_agent_reporter_spans_failures{format="jaeger",protocol="tchannel"} 0
jaeger_agent_reporter_spans_failures{format="zipkin",protocol="tchannel"} 0
jaeger_agent_reporter_spans_submitted{format="jaeger",protocol="tchannel"} 0
jaeger_agent_reporter_spans_submitted{format="zipkin",protocol="tchannel"} 0

jaeger_agent_collector_proxy{endpoint="baggage",protocol="tchannel",result="err"} 0
jaeger_agent_collector_proxy{endpoint="baggage",protocol="tchannel",result="ok"} 0
jaeger_agent_collector_proxy{endpoint="sampling",protocol="tchannel",result="err"} 0
jaeger_agent_collector_proxy{endpoint="sampling",protocol="tchannel",result="ok"} 0
```

- Rename tcollector proxy metric in agent ([#1182](https://github.com/jaegertracing/jaeger/pull/1182), [@pavolloffay](https://github.com/pavolloffay))

The following metric:
```
jaeger_http_server_errors{source="tcollector-proxy",status="5xx"}
```
has been renamed to:
```
jaeger_http_server_errors{source="collector-proxy",status="5xx"}
```

##### New Features

- Add tracegen utility for generating traces ([#1245](https://github.com/jaegertracing/jaeger/pull/1245), [@yurishkuro](https://github.com/yurishkuro))
- Use DCAwareRoundRobinPolicy as fallback for TokenAwarePolicy ([#1285](https://github.com/jaegertracing/jaeger/pull/1285), [@vprithvi](https://github.com/vprithvi))
- Add Zipkin Thrift as kafka ingestion format ([#1256](https://github.com/jaegertracing/jaeger/pull/1256), [@geobeau](https://github.com/geobeau))
- Add `FindTraceID` to the spanstore interface ([#1246](https://github.com/jaegertracing/jaeger/pull/1246), [@vprithvi](https://github.com/vprithvi))
- Migrate from glide to dep ([#1240](https://github.com/jaegertracing/jaeger/pull/1240), [@isaachier](https://github.com/isaachier))
- Make tchannel timeout for reporting in agent configurable ([#1034](https://github.com/jaegertracing/jaeger/pull/1034), [@gouthamve](https://github.com/gouthamve))
- Add archive traces to all-in-one ([#1189](https://github.com/jaegertracing/jaeger/pull/1189), [@pavolloffay](https://github.com/pavolloffay))
- Start moving components of adaptive sampling to OSS ([#973](https://github.com/jaegertracing/jaeger/pull/973), [@black-adder](https://github.com/black-adder))
- Add gRPC communication between agent and collector ([#1165](https://github.com/jaegertracing/jaeger/pull/1165), [#1187](https://github.com/jaegertracing/jaeger/pull/1187), [#1181](https://github.com/jaegertracing/jaeger/pull/1181) and [#1180](https://github.com/jaegertracing/jaeger/pull/1180), [@pavolloffay](https://github.com/pavolloffay))

##### Bug fixes, Minor Improvements

- Update exposed ports in ingester dockerfile ([#1289](https://github.com/jaegertracing/jaeger/pull/1289), [@objectiser](https://github.com/objectiser))
- Upgrade Shopify/Sarama for proper handling newest kafka servers 2.x by ingester ([#1248](https://github.com/jaegertracing/jaeger/pull/1248), [@vprithvi](https://github.com/vprithvi))
- Fix sampling strategies overwriting service entry when no sampling type is specified ([#1244](https://github.com/jaegertracing/jaeger/pull/1244), [@objectiser](https://github.com/objectiser))
- Fix dot replacement for int ([#1272](https://github.com/jaegertracing/jaeger/pull/1272), [@pavolloffay](https://github.com/pavolloffay))
- Add C* query to error logs ([#1250](https://github.com/jaegertracing/jaeger/pull/1250), [@vprithvi](https://github.com/vprithvi))
- Add locking around partitionIDToState map accesses ([#1239](https://github.com/jaegertracing/jaeger/pull/1239), [@vprithvi](https://github.com/vprithvi))
- Reorganize config manager packages in agent ([#1198](https://github.com/jaegertracing/jaeger/pull/1198), [@pavolloffay](https://github.com/pavolloffay))

#### UI Changes

* UI pinned to version 1.0.0. The changelog is available here [v1.0.0](https://github.com/jaegertracing/jaeger-ui/blob/master/CHANGELOG.md#v100-january-18-2019)

1.8.2 (2018-11-28)
------------------

#### UI Changes

##### New Features

- Embedded components (SearchTraces and Tracepage) ([#263](https://github.com/jaegertracing/jaeger/pull/263), [@aljesusg](https://github.com/aljesusg))

##### Bug fixes, Minor Improvements

- Fix link in scatter plot when embed mode ([#283](https://github.com/jaegertracing/jaeger-ui/pull/283), [@aljesusg](https://github.com/aljesusg))
- Fix rendering X axis in TraceResultsScatterPlot - pass milliseconds to moment.js ([#274](https://github.com/jaegertracing/jaeger-ui/pull/274), [@istrel](https://github.com/istrel))


1.8.1 (2018-11-23)
------------------

#### Backend Changes

##### Bug fixes, Minor Improvements

- Make agent timeout for reporting configurable and fix flags overriding ([#1034](https://github.com/jaegertracing/jaeger/pull/1034), [@gouthamve](https://github.com/gouthamve))
- Fix metrics handler registration in agent ([#1178](https://github.com/jaegertracing/jaeger/pull/1178), [@pavolloffay](https://github.com/pavolloffay))


1.8.0 (2018-11-12)
------------------

#### Backend Changes

##### Breaking Changes

- Refactor agent configuration ([#1092](https://github.com/jaegertracing/jaeger/pull/1092), [@pavolloffay](https://github.com/pavolloffay))

The following agent flags has been deprecated in order to support multiple reporters:
```bash
--collector.host-port
--discovery.conn-check-timeout
--discovery.min-peers
```
New flags:
```bash
--reporter.tchannel.host-port
--reporter.tchannel.discovery.conn-check-timeout
--reporter.tchannel.discovery.min-peers
```

- Various changes around metrics produced by jaeger-query: Names scoped to the query component, generated for all span readers (not just ES), consolidate query metrics and include result tag ([#1074](https://github.com/jaegertracing/jaeger/pull/1074), [#1075](https://github.com/jaegertracing/jaeger/pull/1075) and [#1096](https://github.com/jaegertracing/jaeger/pull/1096), [@objectiser](https://github.com/objectiser))

For example, sample of metrics produced for `find_traces` operation before:

```
jaeger_find_traces_attempts 1
jaeger_find_traces_errLatency_bucket{le="0.005"} 0
jaeger_find_traces_errors 0
jaeger_find_traces_okLatency_bucket{le="0.005"} 0
jaeger_find_traces_responses_bucket{le="0.005"} 1
jaeger_find_traces_successes 1
```

And now:

```
jaeger_query_latency_bucket{operation="find_traces",result="err",le="0.005"} 0
jaeger_query_latency_bucket{operation="find_traces",result="ok",le="0.005"} 2
jaeger_query_requests{operation="find_traces",result="err"} 0
jaeger_query_requests{operation="find_traces",result="ok"} 2
jaeger_query_responses_bucket{operation="find_traces",le="0.005"} 2
```

##### New Features

- Configurable deadlock detector interval for ingester ([#1134](https://github.com/jaegertracing/jaeger/pull/1134), [@marqc](https://github.com/marqc))
- Emit spans for elastic storage backend ([#1128](https://github.com/jaegertracing/jaeger/pull/1128), [@annanay25](https://github.com/annanay25))
- Allow to use TLS certificates for Elasticsearch authentication ([#1139](https://github.com/jaegertracing/jaeger/pull/1139), [@clyang82](https://github.com/clyang82))
- Add ingester metrics, healthcheck and rename Kafka cli flags ([#1094](https://github.com/jaegertracing/jaeger/pull/1094), [@ledor473](https://github.com/ledor473))
- Add a metric for number of partitions held ([#1154](https://github.com/jaegertracing/jaeger/pull/1154), [@vprithvi](https://github.com/vprithvi))
- Log jaeger-collector tchannel port ([#1136](https://github.com/jaegertracing/jaeger/pull/1136), [@mindaugasrukas](https://github.com/mindaugasrukas))
- Support tracer env based initialization in hotrod ([#1115](https://github.com/jaegertracing/jaeger/pull/1115), [@eundoosong](https://github.com/eundoosong))
- Publish ingester as binaries and docker image ([#1086](https://github.com/jaegertracing/jaeger/pull/1086), [@ledor473](https://github.com/ledor473))
- Use Go 1.11 ([#1104](https://github.com/jaegertracing/jaeger/pull/1104), [@isaachier](https://github.com/isaachier))
- Tag images with commit SHA and publish to `-snapshot` repository ([#1082](https://github.com/jaegertracing/jaeger/pull/1082), [@pavolloffay](https://github.com/pavolloffay))

##### Bug fixes, Minor Improvements

- Fix child span context while tracing cassandra queries ([#1131](https://github.com/jaegertracing/jaeger/pull/1131), [@annanay25](https://github.com/annanay25))
- Deadlock detector hack for Kafka driver instability ([#1087](https://github.com/jaegertracing/jaeger/pull/1087), [@vprithvi](https://github.com/vprithvi))
- Fix processor overriding data in a buffer ([#1099](https://github.com/jaegertracing/jaeger/pull/1099), [@pavolloffay](https://github.com/pavolloffay))

#### UI Changes

##### New Features

- Span Search - Highlight search results ([#238](https://github.com/jaegertracing/jaeger-ui/pull/238)), [@davit-y](https://github.com/davit-y)
- Span Search - Improve search logic ([#237](https://github.com/jaegertracing/jaeger-ui/pull/237)),  [@davit-y](https://github.com/davit-y)
- Span Search - Add result count, navigation and clear buttons ([#234](https://github.com/jaegertracing/jaeger-ui/pull/234)), [@davit-y](https://github.com/davit-y)

##### Bug Fixes, Minor Improvements

- Use correct duration format for scatter plot ([#266](https://github.com/jaegertracing/jaeger-ui/pull/266)), [@tiffon](https://github.com/tiffon))
- Fix collapse all issues ([#264](https://github.com/jaegertracing/jaeger-ui/pull/264)), [@tiffon](https://github.com/tiffon))
- Use a moderately sized canvas for the span graph ([#257](https://github.com/jaegertracing/jaeger-ui/pull/257)), [@tiffon](https://github.com/tiffon))


1.7.0 (2018-09-19)
------------------

#### UI Changes

- Compare two traces ([#228](https://github.com/jaegertracing/jaeger-ui/pull/228), [@tiffon](https://github.com/tiffon))
- Make tags clickable ([#223](https://github.com/jaegertracing/jaeger-ui/pull/223), [@divdavem](https://github.com/divdavem))
- Directed graph as React component ([#224](https://github.com/jaegertracing/jaeger-ui/pull/224), [@tiffon](https://github.com/tiffon))
- Timeline Expand and Collapse Features ([#221](https://github.com/jaegertracing/jaeger-ui/issues/221), [@davit-y](https://github.com/davit-y))
- Integrate Google Analytics into Search Page ([#220](https://github.com/jaegertracing/jaeger-ui/issues/220), [@davit-y](https://github.com/davit-y))

#### Backend Changes

##### Breaking Changes

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

##### Breaking Changes

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

##### Bug Fixes, Minor Improvements

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

##### Bug Fixes, Minor Improvements

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

##### Bug Fixes, Minor Improvements

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

##### Bug Fixes, Minor Improvements

- Remove the override of GOMAXPROCS (#679) <Cruth kvinc>
- Use UTC timezone for ES indices (#646) <Pavol Loffay>
- Fix elasticsearch create index race condition error (#641) <Pavol Loffay>

#### UI Changes

##### New Features

- Use Ant Design instead of Semantic UI (https://github.com/jaegertracing/jaeger-ui/pull/169)
  - Fix [#164](https://github.com/jaegertracing/jaeger-ui/issues/164) - Use Ant Design instead of Semantic UI
  - Fix [#165](https://github.com/jaegertracing/jaeger-ui/issues/165) - Search results are shown without a date
  - Fix [#69](https://github.com/jaegertracing/jaeger-ui/issues/69) - Missing endpoints in jaeger ui dropdown

##### Bug Fixes, Minor Improvements

- Fix 2 digit lookback (12h, 24h) parsing (https://github.com/jaegertracing/jaeger-ui/issues/167)


1.1.0 (2018-01-03)
------------------

#### Backend Changes

##### New Features

- Add support for retrieving unadjusted/raw traces (#615)
- Add CA certificates to collector/query images (#485)
- Parse zipkin v2 high trace id (#596)

##### Bug Fixes, Minor Improvements

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

##### Bug Fixes, Minor Improvements

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

