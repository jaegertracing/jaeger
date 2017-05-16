# Deployment

*This section is under construction.*

We will soon provide Docker images for individual Jaeger components
[Issue #157](https://github.com/uber/jaeger/pull/157),
as well as orchestration templates, for example to run Jaeger on
[Kubernetes](https://github.com/jaegertracing/jaeger-kubernetes)
and [OpenShift](https://github.com/jaegertracing/jaeger-openshift).

## Agent

Jaeger client libraries expect `jaeger-agent` process to run locally on each host.
The agent exposes the following ports: `5775/udp 6831/udp 6832/udp 5778`.

### Discovery System Integration

The agents can connect point to point to a single collector address, which could be
load balanced by another infrastructure component across multilpe collectors. The agent
can also be configured with a static list of collector addresses.

In the future we will support using a discovery system to dynamically load balance
across several collectors. We use [go-kit](https://github.com/go-kit/kit) to support
a number of different discovery systems, but currently blocked by [Issue 492](https://github.com/go-kit/kit/pull/492).

## Collectors

Collectors require almost no configuration except for the location of Cassandra cluster.
All configuration parameters can be provided via command line options.
At default settings the collector exposes the following ports: `14267 14268`.

```go
go run ./cmd/collector/main.go -h
```

To point collectors to Cassandra cluster, specify `-cassandra.keyspace` and `-cassandra.servers`
options.

Many instances of collectors can be run in parallel.

## Cassandra

Currently Cassandra 3.x is the only storage supported by Jaeger backend.
Before running collectors, the keyspace must be initialized using a script
we provide and Cassandra's interactive shell [`cqlsh`][cqlsh]:

```sh
sh ./plugin/storage/cassandra/cassandra3v001-schema.sh test | cqlsh
```

For production deployment, pass `prod {datacenter}` arguments to the script,
where `{datacenter}` is the name used in the Cassandra configuration.

The script accepts additional parameters as environment variables:

  * `TTL` - default time to live for all data, in seconds (default: 172800, 2 days)
  * `KEYSPACE` - keyspace (default: jaeger_v1_{datacenter})
  * `REPLICATION_FACTOR` - replication factor for prod (default: 2)

## Query Service & UI

Query service requires the location of Cassandra cluster, similar to collectors.
At Uber we run several `jaeger-query` instances behind a single domain managed by nginx.
At default settings the query service exposes the following port(s): `16686`.

## Aggregation Jobs for Service Dependencies

At the moment this is work in progress. We're working on a post-processing data pipeline
that will include aggregating data to present service dependency diagram.


[cqlsh]: http://cassandra.apache.org/doc/latest/tools/cqlsh.html
