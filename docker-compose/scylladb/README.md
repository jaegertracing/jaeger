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

There is a known issue with docker compose network configuration and containers connectivity on Apple silicone. Sometimes it's helpful to manually create the docker network before running `docker compose up`:
```shell
docker network create --driver bridge jaeger-scylladb
```

#### Spin up all infrastructure:

```shell
docker compose up -d
```
Will:
1. Create ScyllaDB cluster with 3 nodes(about 1 min to initialize)
2. Generate the schema for jaeger key space
3. Start Jaeger server, Jaeger UI and Jaeger Demo App `HotRod`

#### Run demo app

1. Wait till all containers are up and running
2. Open Demo app in your browser: http://localhost:8080 and click some buttons.
3. Open Jaeger UI in your browser: http://localhost:16686 and check traces
