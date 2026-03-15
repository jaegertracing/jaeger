# ClickHouse Storage Backend — Benchmark Results

## Overview

This document presents benchmark results for Jaeger's native ClickHouse storage backend.
The benchmarks measure insert throughput, compression efficiency, and query performance
for the most common Jaeger operations: trace retrieval, span search, and metadata lookups.

All benchmark scripts are available in the
[clickhouse-benchmarking](https://github.com/mahadzaryab1/clickhouse-benchmarking) repository.

## Test Environment

| Component | Details |
| --- | --- |
| **VM** | Oracle Cloud VM.Standard2.4 (4 OCPUs / 8 threads, Intel Xeon Platinum 8167M @ 2.0 GHz) |
| **Memory** | 60 GB |
| **Disk** | 30 GB block storage |
| **OS** | Oracle Linux 9 |
| **ClickHouse** | 25.2.1 (single-node, containerized via Podman) |
| **Jaeger** | Built from source (all-in-one, native process) |

## Dataset

| Parameter | Value |
| --- | --- |
| **Total traces** | 1,000,000 |
| **Spans per trace** | 10 (1 parent + 9 children) |
| **Total spans** | 10,000,000 |
| **Services** | 2 (`tracegen-00`, `tracegen-01`) |
| **Partitions (days)** | 5 |
| **Attributes per span** | 11 (across 97 distinct keys, 1000 distinct values) |
| **Generator** | `jaeger-tracegen` via OTLP gRPC |

## Schema

The native schema uses a single [`spans`](sql/create_spans_table.sql) table with `Nested` arrays for attributes:

```sql
ENGINE = MergeTree
PARTITION BY toDate(start_time)
ORDER BY (trace_id)
```

**Skip indexes:**

| Index | Type | Target Column |
| --- | --- | --- |
| `idx_service_name` | `set(500)` | `service_name` |
| `idx_name` | `set(1000)` | `name` (operation) |
| `idx_start_time` | `minmax` | `start_time` |
| `idx_duration` | `minmax` | `duration` |
| `idx_attributes_keys` | `bloom_filter` | `str_attributes.key` |
| `idx_attributes_values` | `bloom_filter` | `str_attributes.value` |
| `idx_resource_attributes_keys` | `bloom_filter` | `resource_str_attributes.key` |
| `idx_resource_attributes_values` | `bloom_filter` | `resource_str_attributes.value` |

## Results

### Compression (`spans` table)

| Metric | Value |
| --- | --- |
| **Uncompressed size** | 5.99 GiB |
| **Compressed size** | 387.79 MiB |
| **Compression ratio** | 15.85x |

Script: [`table_compression_spans`](https://github.com/mahadzaryab1/clickhouse-benchmarking/blob/main/performance-retrieval-scripts/native-schema/table_compression_spans)

### Insert Throughput

| Metric | Value |
| --- | --- |
| **Total spans** | 10,000,000 |
| **Total insert duration** | 308.4 s |
| **Throughput (spans/sec)** | 32,422 |

### Query Performance

Each query was run 3 times. The table shows averages across all runs.

#### Retrieval Queries

These queries fetch already-known data (specific trace IDs, service lists).

| Query | Avg Duration |
| --- | --- |
| [**Retrieve services**](https://github.com/mahadzaryab1/clickhouse-benchmarking/blob/main/setup/native/queries/retrieve_services.sql) | 3 ms |
| [**Retrieve operations**](https://github.com/mahadzaryab1/clickhouse-benchmarking/blob/main/setup/native/queries/retrieve_operations.sql) | 4 ms |
| [**Get trace by ID**](https://github.com/mahadzaryab1/clickhouse-benchmarking/blob/main/setup/native/queries/retrieve_spans_by_trace_id.sql) | 27 ms |
| [**Get trace by ID + time range**](https://github.com/mahadzaryab1/clickhouse-benchmarking/blob/main/setup/native/queries/retrieve_spans_by_trace_id_with_time_range.sql) | 27 ms |

#### Search Queries

These queries search across the spans table with `LIMIT 1000` on distinct trace IDs.

| Query | Avg Duration |
| --- | --- |
| [**Search by service**](https://github.com/mahadzaryab1/clickhouse-benchmarking/blob/main/setup/native/queries/search_by_service.sql) | 43 ms |
| [**Search by operation**](https://github.com/mahadzaryab1/clickhouse-benchmarking/blob/main/setup/native/queries/search_by_operation.sql) | 44 ms |
| [**Search by duration range**](https://github.com/mahadzaryab1/clickhouse-benchmarking/blob/main/setup/native/queries/search_by_duration.sql) | 49 ms |
| [**Search by timestamp range**](https://github.com/mahadzaryab1/clickhouse-benchmarking/blob/main/setup/native/queries/search_by_timestamp.sql) | 46 ms |
| [**Search by attribute**](https://github.com/mahadzaryab1/clickhouse-benchmarking/blob/main/setup/native/queries/search_by_attribute.sql) | 2,451 ms |
| [**Search by all filters**](https://github.com/mahadzaryab1/clickhouse-benchmarking/blob/main/setup/native/queries/search_by_all.sql) | 881 ms |

## Reproducing

See the [clickhouse-benchmarking](https://github.com/mahadzaryab1/clickhouse-benchmarking/tree/main/setup/native) repository for setup and reproduction instructions.
