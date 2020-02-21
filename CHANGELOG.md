Changes by Version
==================

1.17.0 (unreleased)
------------------

### Backend Changes

#### Breaking Changes

#### New Features

#### Bug fixes, Minor Improvements

### UI Changes

1.16.0 (2019-12-17)
------------------

### Backend Changes

#### Breaking Changes

##### List of service operations can be classified by span kinds ([#1943](https://github.com/jaegertracing/jaeger/pull/1943), [#1942](https://github.com/jaegertracing/jaeger/pull/1942), [#1937](https://github.com/jaegertracing/jaeger/pull/1937), [@guo0693](https://github.com/guo0693))

* Endpoint changes:
    * Both Http & gRPC servers now take new optional parameter `spanKind` in addition to `service`. When spanKind
     is absent or empty, operations from all kinds of spans will be returned.
    * Instead of returning a list of string, both Http & gRPC servers return a list of operation struct. Please 
    update your client code to process the new response. Example response:
        ```
        curl 'http://localhost:6686/api/operations?service=UserService&spanKind=server' | jq
        {
            "data": [{
                "name": "UserService::getExtendedUser",
                "spanKind": "server"
            },
            {
                "name": "UserService::getUserProfile",
                "spanKind": "server"
            }],
            "total": 2,
            "limit": 0,
            "offset": 0,
            "errors": null
        }
        ```
    * The legacy http endpoint stay untouched:
        ```
        /services/{%s}/operations
        ```    
* Storage plugin changes:
    * Memory updated to support spanKind on write & read, no migration is required.
    * [Badger](https://github.com/jaegertracing/jaeger/issues/1922) & [ElasticSearch](https://github.com/jaegertracing/jaeger/issues/1923) 
    to be implemented:  
    For now `spanKind` will be set as empty string during read & write, only `name` will be valid operation name.
    * Cassandra updated to support spanKind on write & read ([#1937](https://github.com/jaegertracing/jaeger/pull/1937), [@guo0693](https://github.com/guo0693)):  
        If you don't run the migration script, nothing will break, the system will used the old table 
        `operation_names` and set empty `spanKind` in the response.  
        Steps to get the updated functionality:
        1.  You will need to run the command below on the host where you can use `cqlsh` to connect to Cassandra:
            ```
            KEYSPACE=jaeger_v1 CQL_CMD='cqlsh host 9042 -u test_user -p test_password --request-timeout=3000' 
            bash ./v002tov003.sh
            ```
            The script will create new table `operation_names_v2` and migrate data from the old table.  
            `spanKind` column will be empty for those data.  
            At the end, it will ask you whether you want to drop the old table or not.
        2. Restart ingester & query services so that they begin to use the new table

##### Trace and Span IDs are always padded to 32 or 16 hex characters with leading zeros ([#1956](https://github.com/jaegertracing/jaeger/pull/1956), [@yurishkuro](https://github.com/yurishkuro))

Previously, Jaeger backend always rendered trace and span IDs as  the shortest possible hex string, e.g. an ID
with numeric value 255 would be rendered as a string `ff`. This change makes the IDs to always render as 16 or 32
characters long hex string, e.g. the same id=255 would render as `00000000000000ff`. It mostly affects how UI
displays the IDs, the URLs, and the JSON returned from `jaeger-query` service.

Motivation: Among randomly generated and uniformly distributed trace IDs, only 1/16th of them start with 0
followed by a significant digit, 1/256th start with two 0s, and so on in decreasing geometric progression.
Therefore, trimming the leading 0s is a very modest optimization on the size of the data being transmitted or stored.

However, trimming 0s leads to ambiguities when the IDs are used as correlations with other monitoring systems,
such as logging, that treat the IDs as opaque strings and cannot establish the equivalence between padded and
unpadded IDs. It is also incompatible with W3C Trace Context and Zipkin B3 formats, both of which include all
leading 0s, so an application instrumented with OpenTelemetry SDKs may be logging different trace ID strings
than application instrumented with Jaeger SDKs (related issue #1657).

Overall, the change is backward compatible:
  * links with non-padded IDs in the UI will still work
  * data stored in Elasticsearch (where IDs are represented as strings) is still readable

However, some custom integration that rely on exact string matches of trace IDs may be broken.

##### Change default rollover conditions to 2 days ([#1963](https://github.com/jaegertracing/jaeger/pull/1963), [@pavolloffay](https://github.com/pavolloffay))

Change default rollover conditions from 7 days to 2 days.

Given that by default Jaeger uses daily indices and some organizations do not keep data longer than 7 days
the default of 7 days seems unreasonable - it might result in a too big index and
running curator would immediately remove the old index.

#### New Features

* Support collector tags, similar to agent tags ([#1854](https://github.com/jaegertracing/jaeger/pull/1854), [@radekg](https://github.com/radekg))
* Support insecure TLS and only CA cert for Elasticsearch ([#1918](https://github.com/jaegertracing/jaeger/pull/1918), [@pavolloffay](https://github.com/pavolloffay))
* Allow tracer config via env vars ([#1919](https://github.com/jaegertracing/jaeger/pull/1919), [@yurishkuro](https://github.com/yurishkuro))
* Allow turning off tags/logs indexing in Cassandra ([#1915](https://github.com/jaegertracing/jaeger/pull/1915), [@joe-elliott](https://github.com/joe-elliott))
* Blacklisting/Whitelisting tags for Cassandra indexing  ([#1904](https://github.com/jaegertracing/jaeger/pull/1904), [@joe-elliott](https://github.com/joe-elliott))

#### Bug fixes, Minor Improvements

* Support custom basepath in HotROD ([#1894](https://github.com/jaegertracing/jaeger/pull/1894), [@jan25](https://github.com/jan25))
* Deprecate tchannel reporter flags ([#1978](https://github.com/jaegertracing/jaeger/pull/1978), [@objectiser](https://github.com/objectiser))
* Do not truncate tags in Elasticsearch ([#1970](https://github.com/jaegertracing/jaeger/pull/1970), [@pavolloffay](https://github.com/pavolloffay))
* Export SaveSpan to enable multiplexing ([#1968](https://github.com/jaegertracing/jaeger/pull/1968), [@albertteoh](https://github.com/albertteoh))
* Make rollover init step idempotent ([#1964](https://github.com/jaegertracing/jaeger/pull/1964), [@pavolloffay](https://github.com/pavolloffay))
* Update python urllib3 version required by curator ([#1965](https://github.com/jaegertracing/jaeger/pull/1965), [@pavolloffay](https://github.com/pavolloffay))
* Allow changing max log level for gRPC storage plugins ([#1962](https://github.com/jaegertracing/jaeger/pull/1962), [@yyyogev](https://github.com/yyyogev))
* Fix the bug that operation_name table can not be init more than once ([#1961](https://github.com/jaegertracing/jaeger/pull/1961), [@guo0693](https://github.com/guo0693))
* Improve migration script ([#1946](https://github.com/jaegertracing/jaeger/pull/1946), [@guo0693](https://github.com/guo0693))
* Fix order of the returned results from badger backend.  ([#1939](https://github.com/jaegertracing/jaeger/pull/1939), [@burmanm](https://github.com/burmanm))
* Update python pathlib to pathlib2 ([#1930](https://github.com/jaegertracing/jaeger/pull/1930), [@objectiser](https://github.com/objectiser))
* Use proxy env vars if they're configured ([#1910](https://github.com/jaegertracing/jaeger/pull/1910), [@zoidbergwill](https://github.com/zoidbergwill))

### UI Changes

* UI pinned to version 1.6.0. The changelog is available here [v1.6.0](https://github.com/jaegertracing/jaeger-ui/blob/master/CHANGELOG.md#v160-december-16-2019)

1.15.1 (2019-11-07)
------------------

##### Bug fixes, Minor Improvements

* Build platform binaries as part of CI ([#1909](https://github.com/jaegertracing/jaeger/pull/1909), [@yurishkuro](https://github.com/yurishkuro))
* Upgrade and fix dependencies ([#1907](https://github.com/jaegertracing/jaeger/pull/1907), [@yurishkuro](https://github.com/yurishkuro))


1.15.0 (2019-11-07)
------------------

#### Backend Changes

##### Breaking Changes

* The default value for the Ingester's flag `ingester.deadlockInterval` has been changed to `0` ([#1868](https://github.com/jaegertracing/jaeger/pull/1868), [@jpkrohling](https://github.com/jpkrohling))

  With the new default, the ingester won't `panic` if there are no messages for the last minute. To restore the previous behavior, set the flag's value to `1m`.

* Mark `--collector.grpc.tls.client.ca` flag as deprecated for jaeger-collector. ([#1840](https://github.com/jaegertracing/jaeger/pull/1840), [@yurishkuro](https://github.com/yurishkuro))

  The deprecated flag will still work until being removed, it's recommended to use `--collector.grpc.tls.client-ca` instead.

##### New Features

* Support TLS for Kafka ([#1414](https://github.com/jaegertracing/jaeger/pull/1414), [@MichaHoffmann](https://github.com/MichaHoffmann))
* Add ack and compression parameters for Kafka #1359 ([#1712](https://github.com/jaegertracing/jaeger/pull/1712), [@chandresh-pancholi](https://github.com/chandresh-pancholi))
* Propagate the bearer token to the gRPC plugin server ([#1822](https://github.com/jaegertracing/jaeger/pull/1822), [@radekg](https://github.com/radekg))
* Add Truncate and ReadOnly options for badger ([#1842](https://github.com/jaegertracing/jaeger/pull/1842), [@burmanm](https://github.com/burmanm))

##### Bug fixes, Minor Improvements

* Use correct context on ES search methods ([#1850](https://github.com/jaegertracing/jaeger/pull/1850), [@rubenvp8510](https://github.com/rubenvp8510))
* Handling of expected error codes coming from grpc storage plugins #1741 ([#1814](https://github.com/jaegertracing/jaeger/pull/1814), [@chandresh-pancholi](https://github.com/chandresh-pancholi))
* Fix ordering of indexScanKeys after TraceID parsing ([#1809](https://github.com/jaegertracing/jaeger/pull/1809), [@burmanm](https://github.com/burmanm))
* Small memory optimizations in badger write-path ([#1771](https://github.com/jaegertracing/jaeger/pull/1771), [@burmanm](https://github.com/burmanm))
* Set an empty value when a default env var value is missing ([#1777](https://github.com/jaegertracing/jaeger/pull/1777), [@jpkrohling](https://github.com/jpkrohling))
* Decouple storage dependencies and bump Go to 1.13.x ([#1886](https://github.com/jaegertracing/jaeger/pull/1886), [@yurishkuro](https://github.com/yurishkuro))
* Update gopkg.in/yaml.v2 dependency to v2.2.4 ([#1865](https://github.com/jaegertracing/jaeger/pull/1865), [@objectiser](https://github.com/objectiser))
* Upgrade jaeger-client 2.19 and jaeger-lib 2.2 and prom client 1.x ([#1810](https://github.com/jaegertracing/jaeger/pull/1810), [@yurishkuro](https://github.com/yurishkuro))
* Unpin grpc version and use serviceConfig to set the load balancer  ([#1786](https://github.com/jaegertracing/jaeger/pull/1786), [@guanw](https://github.com/guanw))

#### UI Changes

* UI pinned to version 1.5.0. The changelog is available here [v1.5.0](https://github.com/jaegertracing/jaeger-ui/blob/master/CHANGELOG.md#v150-november-4-2019)

1.14.0 (2019-09-02)
------------------

#### Backend Changes

##### Breaking Changes

* Create ES index templates instead of indices ([#1627](https://github.com/jaegertracing/jaeger/pull/1627), [@pavolloffay](https://github.com/pavolloffay))

  This can break existing Elasticsearch deployments if security policies are applied.
  For instance Jaeger `X-Pack` configuration now requires permission to create index templates - `manage_index_templates`.

##### New Features

* Add Elasticsearch version configuration to rollover script ([#1769](https://github.com/jaegertracing/jaeger/pull/1769), [@pavolloffay](https://github.com/pavolloffay))
* Add Elasticsearch version flag ([#1753](https://github.com/jaegertracing/jaeger/pull/1753), [@pavolloffay](https://github.com/pavolloffay))
* Add Elasticsearch 7 support ([#1690](https://github.com/jaegertracing/jaeger/pull/1690), [@gregoryfranklin](https://github.com/gregoryfranklin))

  The index mappings in Elasticsearch 7 are not backwards compatible with the older versions.
  Therefore using Elasticsearch 7 with data created with older version would not work.
  Elasticsearch 6.8 supports 7.x, 6.x, 5.x compatible mappings. The upgrade has to be done
  first to ES 6.8, then apply data migration or wait until old daily indices are removed (this requires
  to start Jaeger with `--es.version=7` to force using ES 7.x mappings for newly created indices).

  Jaeger by default uses Elasticsearch ping endpoint (`/`) to derive the version which is used
  for index mappings selection. The version can be overridden by flag `--es.version`.

* Support for Zipkin Protobuf spans over HTTP ([#1695](https://github.com/jaegertracing/jaeger/pull/1695), [@jan25](https://github.com/jan25))
* Added support for hot reload of UI config ([#1688](https://github.com/jaegertracing/jaeger/pull/1688), [@jpkrohling](https://github.com/jpkrohling))
* Added base Grafana dashboard and Alert rules ([#1745](https://github.com/jaegertracing/jaeger/pull/1745), [@jpkrohling](https://github.com/jpkrohling))
* Add the jaeger-mixin for monitoring ([#1668](https://github.com/jaegertracing/jaeger/pull/1668), [@gouthamve](https://github.com/gouthamve))
* Added flags for driving cassandra connection compression through config ([#1675](https://github.com/jaegertracing/jaeger/pull/1675), [@sagaranand015](https://github.com/sagaranand015))
* Support index cleaner for rollover indices and add integration tests ([#1689](https://github.com/jaegertracing/jaeger/pull/1689), [@pavolloffay](https://github.com/pavolloffay))
* Add client TLS auth to gRPC reporter ([#1591](https://github.com/jaegertracing/jaeger/pull/1591), [@tcolgate](https://github.com/tcolgate))
* Collector kafka producer protocol version config ([#1658](https://github.com/jaegertracing/jaeger/pull/1658), [@marqc](https://github.com/marqc))
* Configurable kafka protocol version for msg consuming by jaeger ingester ([#1640](https://github.com/jaegertracing/jaeger/pull/1640), [@marqc](https://github.com/marqc))
* Use credentials when describing keyspaces in cassandra schema builder ([#1655](https://github.com/jaegertracing/jaeger/pull/1655), [@MiLk](https://github.com/MiLk))
* Add connect-timeout for Cassandra ([#1647](https://github.com/jaegertracing/jaeger/pull/1647), [@sagaranand015](https://github.com/sagaranand015))

##### Bug fixes, Minor Improvements

* Fix gRPC over cmux and add unit tests ([#1758](https://github.com/jaegertracing/jaeger/pull/1758), [@yurishkuro](https://github.com/yurishkuro))
* Add CA certificates to agent image ([#1764](https://github.com/jaegertracing/jaeger/pull/1764), [@yurishkuro](https://github.com/yurishkuro))
* Fix badger merge-join algorithm to correctly filter indexes ([#1721](https://github.com/jaegertracing/jaeger/pull/1721), [@burmanm](https://github.com/burmanm))
* Change Zipkin CORS origins and headers to comma separated list ([#1556](https://github.com/jaegertracing/jaeger/pull/1556), [@JonasVerhofste](https://github.com/JonasVerhofste))
* Added null guards to 'Process' when processing an incoming span ([#1723](https://github.com/jaegertracing/jaeger/pull/1723), [@jpkrohling](https://github.com/jpkrohling))
* Export expvar metrics of badger to the metricsFactory ([#1704](https://github.com/jaegertracing/jaeger/pull/1704), [@burmanm](https://github.com/burmanm))
* Pass TTL as int, not as float64 ([#1710](https://github.com/jaegertracing/jaeger/pull/1710), [@yurishkuro](https://github.com/yurishkuro))
* Use find by regex for archive index in index cleaner ([#1693](https://github.com/jaegertracing/jaeger/pull/1693), [@pavolloffay](https://github.com/pavolloffay))
* Allow token propagation if token type is not specified ([#1685](https://github.com/jaegertracing/jaeger/pull/1685), [@rubenvp8510](https://github.com/rubenvp8510))
* Fix duplicated spans when querying Elasticsearch ([#1677](https://github.com/jaegertracing/jaeger/pull/1677), [@pavolloffay](https://github.com/pavolloffay))
* Fix the threshold precision issue ([#1665](https://github.com/jaegertracing/jaeger/pull/1665), [@guanw](https://github.com/guanw))
* Badger filter duplicate results from a single indexSeek ([#1649](https://github.com/jaegertracing/jaeger/pull/1649), [@burmanm](https://github.com/burmanm))
* Badger make default dirs work in Windows ([#1653](https://github.com/jaegertracing/jaeger/pull/1653), [@burmanm](https://github.com/burmanm))

#### UI Changes

* UI pinned to version 1.4.0. The changelog is available here [v1.4.0](https://github.com/jaegertracing/jaeger-ui/blob/master/CHANGELOG.md#v130-june-21-2019)

1.13.1 (2019-06-28)
------------------

#### Backend Changes

##### Breaking Changes

##### New Features

##### Bug fixes, Minor Improvements

* Change default for bearer-token-propagation to false ([#1642](https://github.com/jaegertracing/jaeger/pull/1642), [@wsoula](https://github.com/wsoula))

#### UI Changes

1.13.0 (2019-06-27)
------------------

#### Backend Changes

##### Breaking Changes

* The traces related metrics on collector now have a new tag `sampler_type` ([#1576](https://github.com/jaegertracing/jaeger/pull/1576), [@guanw](https://github.com/guanw))

  This might break some existing metrics dashboard (if so, users need to update query to aggregate over this new tag).

  The list of metrics affected: `traces.received`, `traces.rejected`, `traces.saved-by-svc`.

* Remove deprecated index prefix separator `:` from Elastic ([#1620](https://github.com/jaegertracing/jaeger/pull/1620), [@pavolloffay](https://github.com/pavolloffay))

  In Jaeger 1.9.0 release the Elasticsearch index separator was changed from `:` to `-`. To keep backwards
  compatibility the query service kept querying indices with `:` separator, however the new indices
  were created only with `-`. This release of Jaeger removes the query capability for indices containing `:`,
  therefore it's recommended to keep using older version until indices containing old separator are
  not queried anymore.

##### New Features

* Passthrough OAuth bearer token supplied to Query service through to ES storage ([#1599](https://github.com/jaegertracing/jaeger/pull/1599), [@rubenvp8510](https://github.com/rubenvp8510))
* Kafka kerberos authentication support for collector/ingester ([#1589](https://github.com/jaegertracing/jaeger/pull/1589), [@rubenvp8510](https://github.com/rubenvp8510))
* Allow Cassandra schema builder to use credentials ([#1635](https://github.com/jaegertracing/jaeger/pull/1635), [@PS-EGHornbostel](https://github.com/PS-EGHornbostel))
* Add docs generation command ([#1572](https://github.com/jaegertracing/jaeger/pull/1572), [@pavolloffay](https://github.com/pavolloffay))

##### Bug fixes, Minor Improvements

* Fix data race between `Agent.Run()` and `Agent.Stop()` ([#1625](https://github.com/jaegertracing/jaeger/pull/1625), [@tigrannajaryan](https://github.com/tigrannajaryan))
* Use json number when unmarshalling data from ES ([#1618](https://github.com/jaegertracing/jaeger/pull/1618), [@pavolloffay](https://github.com/pavolloffay))
* Define logs as nested data type ([#1622](https://github.com/jaegertracing/jaeger/pull/1622), [@pavolloffay](https://github.com/pavolloffay))
* Fix archive storage not querying old spans older than maxSpanAge ([#1617](https://github.com/jaegertracing/jaeger/pull/1617), [@pavolloffay](https://github.com/pavolloffay))
* Query service: fix logging errors on SIGINT ([#1601](https://github.com/jaegertracing/jaeger/pull/1601), [@jan25](https://github.com/jan25))
* Direct grpc logs to Zap logger ([#1606](https://github.com/jaegertracing/jaeger/pull/1606), [@yurishkuro](https://github.com/yurishkuro))
* Fix sending status to health check channel in Query service ([#1598](https://github.com/jaegertracing/jaeger/pull/1598), [@jan25](https://github.com/jan25))
* Add tmp-volume to all-in-one image to fix badger storage ([#1571](https://github.com/jaegertracing/jaeger/pull/1571), [@burmanm](https://github.com/burmanm))
* Do not fail es-cleaner if there are no jaeger indices ([#1569](https://github.com/jaegertracing/jaeger/pull/1569), [@pavolloffay](https://github.com/pavolloffay))
* Automatically set `GOMAXPROCS` ([#1560](https://github.com/jaegertracing/jaeger/pull/1560), [@rubenvp8510](https://github.com/rubenvp8510))
* Add CA certs to all-in-one image ([#1554](https://github.com/jaegertracing/jaeger/pull/1554), [@chandresh-pancholi](https://github.com/chandresh-pancholi))

#### UI Changes

* UI pinned to version 1.3.0. The changelog is available here [v1.3.0](https://github.com/jaegertracing/jaeger-ui/blob/master/CHANGELOG.md#v130-june-21-2019)

1.12.0 (2019-05-16)
------------------

#### Backend Changes

##### Breaking Changes
- The `kafka` flags were removed in favor of `kafka.producer` and `kafka.consumer` flags ([#1424](https://github.com/jaegertracing/jaeger/pull/1424), [@ledor473](https://github.com/ledor473))

    The following flags have been **removed** in the Collector and the Ingester:
    ```
    --kafka.brokers
    --kafka.encoding
    --kafka.topic
    --ingester.brokers
    --ingester.encoding
    --ingester.topic
    --ingester.group-id
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
    --kafka.consumer.topic
    --kafka.consumer.group-id
    ```

* Add Admin port and group all ports in one file ([#1442](https://github.com/jaegertracing/jaeger/pull/1442), [@yurishkuro](https://github.com/yurishkuro))

    This change fixes issues [#1428](https://github.com/jaegertracing/jaeger/issues/1428), [#1332](https://github.com/jaegertracing/jaeger/issues/1332) and moves all metrics endpoints from API ports to **admin ports**. It requires re-configuring Prometheus scraping rules. Each Jaeger binary has its own admin port that can be found under `--admin-http-port` command line flag by running the `${binary} help` command.

##### New Features

* Add gRPC resolver using external discovery service ([#1498](https://github.com/jaegertracing/jaeger/pull/1498), [@guanw](https://github.com/guanw))
* gRPC storage plugin framework ([#1461](https://github.com/jaegertracing/jaeger/pull/1461), [@chvck](https://github.com/chvck))
* Supports customized kafka client id ([#1507](https://github.com/jaegertracing/jaeger/pull/1507), [@newly12](https://github.com/newly12))
* Support gRPC for query service ([#1307](https://github.com/jaegertracing/jaeger/pull/1307), [@annanay25](https://github.com/annanay25))
* Expose tls.InsecureSkipVerify to es.tls.* CLI flags ([#1473](https://github.com/jaegertracing/jaeger/pull/1473), [@stefanvassilev](https://github.com/stefanvassilev))
* Return info msg for `/health` endpoint ([#1465](https://github.com/jaegertracing/jaeger/pull/1465), [@stefanvassilev](https://github.com/stefanvassilev))
* Add pprof endpoint to admin endpoint ([#1375](https://github.com/jaegertracing/jaeger/pull/1375), [@konradgaluszka](https://github.com/konradgaluszka))
* Add inbound transport as label to collector metrics [#1446](https://github.com/jaegertracing/jaeger/pull/1446) ([guanw](https://github.com/guanw))
* Sorted key/value store `badger` backed storage plugin ([#760](https://github.com/jaegertracing/jaeger/pull/760), [@burmanm](https://github.com/burmanm))
* Add Admin port and group all ports in one file ([#1442](https://github.com/jaegertracing/jaeger/pull/1442), [@yurishkuro](https://github.com/yurishkuro))
* Adds support for agent level tag ([#1396](https://github.com/jaegertracing/jaeger/pull/1396), [@annanay25](https://github.com/annanay25))
* Add a Downsampling writer that drop a percentage of spans ([#1353](https://github.com/jaegertracing/jaeger/pull/1353), [@guanw](https://github.com/guanw))

##### Bug fixes, Minor Improvements

* Sort traces in memory store to return most recent traces ([#1394](https://github.com/jaegertracing/jaeger/pull/1394), [@jacobmarble](https://github.com/jacobmarble))
* Add span format tag for jaeger-collector ([#1493](https://github.com/jaegertracing/jaeger/pull/1493), [@guo0693](https://github.com/guo0693))
* Upgrade gRPC to 1.20.1 ([#1492](https://github.com/jaegertracing/jaeger/pull/1492), [@guanw](https://github.com/guanw))
* Switch from counter to a gauge for partitions held ([#1485](https://github.com/jaegertracing/jaeger/pull/1485), [@bobrik](https://github.com/bobrik))
* Add CORS handling for Zipkin collector service ([#1463](https://github.com/jaegertracing/jaeger/pull/1463), [@JonasVerhofste](https://github.com/JonasVerhofste))
* Check elasticsearch nil response ([#1467](https://github.com/jaegertracing/jaeger/pull/1467), [@YEXINGZHE54](https://github.com/YEXINGZHE54))
* Disable sampling in logger - `zap`([#1460](https://github.com/jaegertracing/jaeger/pull/1460), [@psinghal20](https://github.com/psinghal20))
* New layout for proto definitions and generated files ([#1427](https://github.com/jaegertracing/jaeger/pull/1427), [@annanay25](https://github.com/annanay25))
* Upgrade Go to 1.12.1 ([#1437](https://github.com/jaegertracing/jaeger/pull/1437) ,[@yurishkuro](https://github.com/yurishkuro))

#### UI Changes

* UI pinned to version 1.2.0. The changelog is available here [v1.2.0](https://github.com/jaegertracing/jaeger-ui/blob/master/CHANGELOG.md#v120-may-14-2019)

1.11.0 (2019-03-07)
------------------

#### Backend Changes

##### Breaking Changes
- Introduce `kafka.producer` and `kafka.consumer` flags to replace `kafka` flags ([#1360](https://github.com/jaegertracing/jaeger/pull/1360), [@ledor473](https://github.com/ledor473))

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
- Allow to use TLS with ES basic auth ([#1388](https://github.com/jaegertracing/jaeger/pull/1388), [@pavolloffay](https://github.com/pavolloffay))

##### Bug fixes, Minor Improvements

- Make `esRollover.py init` idempotent ([#1407](https://github.com/jaegertracing/jaeger/pull/1407) and [#1408](https://github.com/jaegertracing/jaeger/pull/1408), [@pavolloffay](https://github.com/pavolloffay))
- Allow thrift reporter if grpc hosts are not provided ([#1400](https://github.com/jaegertracing/jaeger/pull/1400), [@pavolloffay](https://github.com/pavolloffay))
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

