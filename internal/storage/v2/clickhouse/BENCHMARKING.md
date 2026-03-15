# ClickHouse Storage Backend — Benchmarks

## Test Environment

| Component | Details |
| --- | --- |
| **VM** | Oracle Cloud VM.Standard2.4 (4 OCPUs, Intel Xeon Platinum 8167M) |
| **Memory** | 60 GB |
| **Disk** | 47 GB block storage |
| **OS** | Oracle Linux 9 |
| **ClickHouse** | 26 (single-node) |

## Dataset

| Parameter | Value |
| --- | --- |
| **Total traces** | 1,000,000 |
| **Spans per trace** | 10 (1 parent + 9 children) |
| **Total spans** | 10,000,000 |
| **Services** | 2 |
| **Partitions (days)** | 5 |
| **Attributes per span** | 11 (across 97 distinct keys, 1000 distinct values) |

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

Script: [`schema_insert`](https://github.com/mahadzaryab1/clickhouse-benchmarking/blob/main/performance-retrieval-scripts/native-schema/schema_insert)

Each query was run 3 times. The table shows averages across all runs.

#### Retrieval Queries

| Query | Avg Duration |
| --- | --- |
| [**Retrieve services**](https://github.com/mahadzaryab1/clickhouse-benchmarking/blob/main/setup/native/queries/retrieve_services.sql) | 3 ms |
| [**Retrieve operations**](https://github.com/mahadzaryab1/clickhouse-benchmarking/blob/main/setup/native/queries/retrieve_operations.sql) | 4 ms |
| [**Get trace by ID**](https://github.com/mahadzaryab1/clickhouse-benchmarking/blob/main/setup/native/queries/retrieve_spans_by_trace_id.sql) | 27 ms |
| [**Get trace by ID + time range**](https://github.com/mahadzaryab1/clickhouse-benchmarking/blob/main/setup/native/queries/retrieve_spans_by_trace_id_with_time_range.sql) | 27 ms |

#### Search Queries

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
