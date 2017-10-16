# Deployment

The main Jaeger backend components are released as Docker images on Docker Hub:

Component             | Repository
--------------------- | ---
**jaeger-agent**      | [hub.docker.com/r/jaegertracing/jaeger-agent/](https://hub.docker.com/r/jaegertracing/jaeger-agent/)
**jaeger-collector**  | [hub.docker.com/r/jaegertracing/jaeger-collector/](https://hub.docker.com/r/jaegertracing/jaeger-collector/)
**jaeger-query**      | [hub.docker.com/r/jaegertracing/jaeger-query/](https://hub.docker.com/r/jaegertracing/jaeger-query/)

There are orchestration templates for running Jaeger with:

  * Kubernetes: [github.com/jaegertracing/jaeger-kubernetes](https://github.com/jaegertracing/jaeger-kubernetes),
  * OpenShift: [github.com/jaegertracing/jaeger-openshift](https://github.com/jaegertracing/jaeger-openshift).

## Storage Backend

<<<<<<< HEAD
Collectors require a persistent storage backend. Cassandra 3.4+ (default) and ElasticSearch are
primary supported storage backends. 
=======
Collectors require a persistent storage backend. Cassandra 3.x (default) and ElasticSearch are the
primary supported storage backends. There is ongoing work to add support for MySQL and ScyllaDB.
>>>>>>> d69b82dc8b19099c2044448512f2c0708efeb5cc

### Cassandra

A script is provided to initialize Cassandra keyspace and schema
<<<<<<< HEAD
using Cassandra's interactive shell [`cqlsh`][cqlsh]

```sh
MODE=test sh ./plugin/storage/cassandra/schema/create.sh | cqlsh
```
or if you don't have the source code 
```
docker run -e MODE=... jaegertracing/jaeger-cassandra-schema
=======
using Cassandra's interactive shell [`cqlsh`][cqlsh],
clone the source repository following [getting
started](https://github.com/jaegertracing/jaeger/blob/master/CONTRIBUTING.md#getting-started) and then run:

```sh
MODE=test sh ./plugin/storage/cassandra/schema/create.sh | cqlsh
>>>>>>> d69b82dc8b19099c2044448512f2c0708efeb5cc
```
For production deployment, pass `MODE=prod DATACENTER={datacenter}` arguments to the script,
where `{datacenter}` is the name used in the Cassandra configuration / network topology.

<<<<<<< HEAD
The script also allows overriding TTL, keyspace name, replication factor, etc.
Run the script without arguments to see the full list of recognized parameters.

### ElasticSearch

ElasticSearch does not require initialization other than
[installing and running ElasticSearch](https://www.elastic.co/downloads/elasticsearch).
Once it is running, pass the correct configuration values to the Jaeger collector and query service.

#### Shards and Replicas for ElasticSearch indices

=======
For production deployment, pass `MODE=prod DATACENTER={datacenter}` arguments to the script,
where `{datacenter}` is the name used in the Cassandra configuration / network topology.

The script also allows overriding TTL, keyspace name, replication factor, etc.
Run the script without arguments to see the full list of recognized parameters.

### ElasticSearch

ElasticSearch does not require initialization other than
[installing and running ElasticSearch](https://www.elastic.co/downloads/elasticsearch).
Once it is running, pass the correct configuration values to the Jaeger collector and query service.

#### Shards and Replicas for ElasticSearch indices

>>>>>>> d69b82dc8b19099c2044448512f2c0708efeb5cc
Shards and replicas are some configuration values to take special attention to, because this is decided upon
index creation. [This article](https://qbox.io/blog/optimizing-elasticsearch-how-many-shards-per-index) goes into
more information about choosing how many shards should be chosen for optimization.

## Collectors

Many instances of **jaeger-collector** can be run in parallel.
Collectors require almost no configuration, except for the location of Cassandra cluster,
via `--cassandra.keyspace` and `--cassandra.servers` options, or the location of ElasticSearch cluster, via
`--es.server-urls`, depending on which storage is specified. To see all command line options run

```
go run ./cmd/collector/main.go -h
```

or, if you don't have the source code

```
docker run -it --rm jaegertracing/jaeger-collector /go/bin/collector-linux -h
```
Example:
```
docker run -it --rm -p14267:14267 -p14268:14268
jaegertracing/jaeger-collector /go/bin/collector-linux
--cassandra.keyspace=jaeger_v1_test --cassandra.servers=192.168.0.183
```
At default settings the collector exposes the following ports:

Port  | Protocol | Function
----- | -------  | ---
14267 | TChannel | used by **jaeger-agent** to send spans in jaeger.thrift format
14268 | HTTP     | can accept spans directly from clients in jaeger.thrift format
9411  | HTTP     | can accept Zipkin spans in JSON or Thrift (disabled by default)


## Agent

Jaeger client libraries expect **jaeger-agent** process to run locally on each host.
The agent exposes the following ports:

Port | Protocol | Function
---- | -------  | ---
5775 | UDP      | accept zipkin.thrift over compact thrift protocol
6831 | UDP      | accept jaeger.thrift over compact thrift protocol
6832 | UDP      | accept jaeger.thrift over binary thrift protocol
5778 | HTTP     | serve configs, sampling strategies

It can be executed directly on the host or via Docker, as follows:

```bash
## make sure to expose only the ports you use in your deployment scenario!
docker run \
  --rm \
  -p5775:5775/udp \
  -p6831:6831/udp \
  -p6832:6832/udp \
  -p5778:5778/tcp \
  jaegertracing/jaeger-agent
  /go/bin/agent-linux --collector.host-port=jaeger-collector.jaeger-infra.svc:14267
```

### Discovery System Integration

The agents can connect point to point to a single collector address, which could be
load balanced by another infrastructure component (e.g. DNS) across multiple collectors.
The agent can also be configured with a static list of collector addresses.

On Docker, a command like the following can be used:

```bash
docker run \
  --rm \
  -p5775:5775/udp \
  -p6831:6831/udp \
  -p6832:6832/udp \
  -p5778:5778/tcp \
  jaegertracing/jaeger-agent \
  /go/bin/agent-linux --collector.host-port=jaeger-collector.jaeger-infra.svc:14267
```

In the future we will support different service discovery systems to dynamically load balance
across several collectors ([issue 213](https://github.com/uber/jaeger/issues/213)).

## Query Service & UI

**jaeger-query** serves the API endpoints and a React/Javascript UI.
The service is stateless and is typically run behind a load balancer, e.g. nginx.

An example to test Query Service:
```
docker run -it -p16686:16686 jaegertracing/jaeger-query:latest
/go/bin/query-linux --cassandra.keyspace=jaeger_v1_test
--cassandra.servers=192.168.0.183
```
At default settings the query service exposes the following port(s):

Port  | Protocol | Function
----- | -------  | ---
16686 | HTTP     | **/api/*** endpoints and Jaeger UI at **/**

TODO: Swagger and GraphQL API ([issue 158](https://github.com/uber/jaeger/issues/158)).

## Aggregation Jobs for Service Dependencies

Production deployments need an external process which aggregates data and creates dependency links between services.
Project [spark-dependencies](https://github.com/jaegertracing/spark-dependencies) is a Spark job which derives
dependency links and stores them directly to the storage.

## Configuration
All binaries accepts command line properties and environmental variables which are managed by
by [viper](https://github.com/spf13/viper) and [cobra](https://github.com/spf13/cobra).
The names of environmental properties are capital letters and characters `-` and `.` are replaced with `_`.
To list all configuration properties call `jaeger-binary -h`.

[cqlsh]: http://cassandra.apache.org/doc/latest/tools/cqlsh.html
