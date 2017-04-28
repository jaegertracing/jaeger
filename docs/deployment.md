# Deployment

*This section is under construction*

We will soon provide Docker images for individual Jaeger components,
as well as orchestration configurations, for example to run them on Kubernetes.

## Agent

Jaeger client libraries expect `jaeger-agent` process to run locally on each host.

### Discovery System Integration

The agents can connect point to point to a single collector,
or use a discovery system to load balance across several collectors.
We use [go-kit](https://github.com/go-kit/kit) to support a number of different
discovery systems, but currently blocked by [Issue 492](https://github.com/go-kit/kit/pull/492).

TODO: [#134: allow agents to connect to multiple collectors via static list](https://github.com/uber/jaeger/issues/134)

## Collectors

Collectors require almost no configuration except for the location of Cassandra cluster.
All configuration parameters can be provided via command line options.

```go
go run ./cmd/collector/main.go -h
```

To point collectors to Cassandra cluster, specify `-cassandra.keyspace` and `-cassandra.servers`
options.

Many instances of collectors can be run in parallel.

## Cassandra

Currently Cassandra 3.x is the only storage supported by Jaeger backend.
Before running collectors, the keyspace must be initialized using a script we provide:

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
