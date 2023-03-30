## ScyllaDB as a storage backend
Jaeger could be configured to use ScyllaDB as a storage backend. This is an experimental feature and this is not an officially supported backend, meaning that Jaeger team will not proactively address any issues that may arise from incompatibilities between the ScyllaDB and Cassandra databases (the team may still accept PRs).

### Configuration

Setup Jaeger server to use Cassandra database and just replace conn string to ScyllaDB cluster. No additional configuration is required.

### Known issues

#### Protocol version

Jaeger server detects Cassandra protocol version automatically. At the date of the demo with specified versions server detects that it connected via protocol version 3 while it is actually 4. This leads to warn log in cassandra-schema container:
```
WARN: DESCRIBE|DESC was moved to server side in Cassandra 4.0. As a consequence DESRIBE|DESC will not work in cqlsh '6.0.0' connected to Cassandra '3.0.8', the version that you are connected to. DESCRIBE does not exist server side prior Cassandra 4.0.
Cassandra version detected: 3
```

Otherwise, it should be fully compatible.

### Demo

Docker compose file consists of Jaeger server, Jaeger Cassandra schema writer, Jaeger UI, Jaeger Demo App `HotRod` and a ScyllaDB cluster.

There is known issues with docker compose network configuration and containers connectivity on Apple Silicone. That's why before `upping` the docker compose you need to manually create network which later be used by docker compose:
```shell
docker network create --driver bridge jaeger-scylladb
```

Create ScyllaDB cluster with 3 nodes and give it some time to start(around 1 minute):
```shell
docker compose up -d scylladb scylladb2 scylladb3
```

Start Jaeger server, Jaeger UI, Jaeger Demo App `HotRod` and Jaeger Cassandra schema writer:
```shell
docker compose up -d
```
Open Demo app in your browser: http://localhost:8080 and make some clicks 

Open Jaeger UI in your browser: http://localhost:16686 and check traces
