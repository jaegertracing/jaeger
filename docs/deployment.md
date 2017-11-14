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
across several collectors ([issue 213](https://github.com/jaegertracing/jaeger/issues/213)).

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

At default settings the collector exposes the following ports:

Port  | Protocol | Function
----- | -------  | ---
14267 | TChannel | used by **jaeger-agent** to send spans in jaeger.thrift format
14268 | HTTP     | can accept spans directly from clients in jaeger.thrift format
9411  | HTTP     | can accept Zipkin spans in JSON or Thrift (disabled by default)


## Storage Backend

Collectors require a persistent storage backend. Cassandra 3.x (default) and ElasticSearch are the
primary supported storage backends. There is ongoing work to add support for MySQL and ScyllaDB.

### Cassandra

A script is provided to initialize Cassandra keyspace and schema
using Cassandra's interactive shell [`cqlsh`][cqlsh]:

```sh
MODE=test sh ./plugin/storage/cassandra/schema/create.sh | cqlsh
```

For production deployment, pass `MODE=prod DATACENTER={datacenter}` arguments to the script,
where `{datacenter}` is the name used in the Cassandra configuration / network topology.

The script also allows overriding TTL, keyspace name, replication factor, etc.
Run the script without arguments to see the full list of recognized parameters.

### ElasticSearch

ElasticSearch does not require initialization other than
[installing and running ElasticSearch](https://www.elastic.co/downloads/elasticsearch).
Once it is running, pass the correct configuration values to the Jaeger collector and query service.

#### Shards and Replicas for ElasticSearch indices

Shards and replicas are some configuration values to take special attention to, because this is decided upon
index creation. [This article](https://qbox.io/blog/optimizing-elasticsearch-how-many-shards-per-index) goes into
more information about choosing how many shards should be chosen for optimization.

## Query Service & UI

**jaeger-query** serves the API endpoints and a React/Javascript UI.
The service is stateless and is typically run behind a load balancer, e.g. nginx.

At default settings the query service exposes the following port(s):

Port  | Protocol | Function
----- | -------  | ---
16686 | HTTP     | **/api/*** endpoints and Jaeger UI at **/**

### UI Configuration

Two aspects of the UI can be configured:

  * The top-right menu in the global nav
  * A Google Analytics ID can be defined to enable Google Analytics tracking in the UI

These options can be configured by a JSON configuration file. The `--query.ui-config` command line parameter of the query service must then be set to the path to the JSON file when the query service is started.

An example configuration file:

```json
{
  "gaTrackingID": " UA-000000-2",
  "menu": [
    {
      "label": "About Jaeger",
      "items": [
        {
          "label": "GitHub",
          "url": "https://github.com/jaegertracing/jaeger"
        },
        {
          "label": "Docs",
          "url": "http://jaeger.readthedocs.io/en/latest/"
        }
      ]
    }
  ]
}
```

In the above example, `gaTrackingID` will be used as the Google Analytics tracking ID and `menu` configures the menu in the top right of the UI.

The configured menu will have a dropdown labeled "About Jaeger" with sub-options for "GitHub" and "Docs". The format for a link in the top right menu is as follows:

```json
{
  "label": "Some text here",
  "url": "https://example.com"
}
```

Links can either be members of the `menu` Array, directly, or they can be grouped into a dropdown menu option. The format for a group of links is:

```json
{
  "label": "Dropdown button",
  "items": [ ]
}
```

The `items` Array should contain one or more link configurations.

TODO: Swagger and GraphQL API ([issue 158](https://github.com/jaegertracing/jaeger/issues/158)).

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
