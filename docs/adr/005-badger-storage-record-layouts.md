# Badger Storage Record Layouts

* **Status**: Documented existing implementation
* **Date**: 2026-03-12

## Context

Jaeger supports [Badger](https://github.com/dgraph-io/badger) as an embedded, local key-value store backend. Badger is primarily intended for all-in-one deployments where a lightweight storage solution is desirable without an external database dependency. Because Badger is a generic sorted key-value store, Jaeger must impose its own logical record structure on top of it.

This ADR documents the record layouts used in the Badger storage implementation as they exist today, covering key formats, value formats, and the overall design rationale. The intent is to make the storage design visible to contributors and to serve as a reference when reasoning about query behavior or considering future changes.

The implementation lives in:
- [`internal/storage/v1/badger/spanstore/writer.go`](../../internal/storage/v1/badger/spanstore/writer.go) — key generation and span writes
- [`internal/storage/v1/badger/spanstore/reader.go`](../../internal/storage/v1/badger/spanstore/reader.go) — query execution and span reads
- [`internal/storage/v1/badger/spanstore/cache.go`](../../internal/storage/v1/badger/spanstore/cache.go) — in-memory service/operation cache
- [`internal/storage/v1/badger/samplingstore/storage.go`](../../internal/storage/v1/badger/samplingstore/storage.go) — sampling data storage
- [`internal/storage/v1/badger/config.go`](../../internal/storage/v1/badger/config.go) — configuration and defaults

### Design Principles

All keys are encoded in **big-endian** byte order. This ensures that integer values sort lexicographically in the same order as their numeric values, which is a prerequisite for the range-scan and reverse-iteration queries used throughout the implementation.

All span-related records (primary span records and all secondary indexes) share a common property: **the most significant bit of the first byte is always set** (`0x80` or higher). This cleanly separates span data from sampling data (prefixes `0x08` and `0x09`) in the keyspace.

A single Badger database instance holds all record types. There is no separate "index store" — different logical tables are distinguished solely by the prefix byte of their keys.

---

## Record Layouts

### Key Prefix Summary

| Record type            | Prefix byte | Used by          |
|------------------------|-------------|------------------|
| Span (primary record)  | `0x80`      | Span store       |
| Service name index     | `0x81`      | Span store       |
| Operation name index   | `0x82`      | Span store       |
| Tag index              | `0x83`      | Span store       |
| Duration index         | `0x84`      | Span store       |
| Throughput             | `0x08`      | Sampling store   |
| Probabilities/QPS      | `0x09`      | Sampling store   |

---

### 1. Primary Span Record (`0x80`)

Each span is stored as a single key-value entry.

**Key** (33 bytes, fixed size):
```
[0x80][traceID.High: 8B][traceID.Low: 8B][startTime: 8B][spanID: 8B]
```

- `traceID.High` and `traceID.Low` — the 128-bit trace ID split into two `uint64` values
- `startTime` — `uint64`, microseconds since Unix epoch (via `model.TimeAsEpochMicroseconds`)
- `spanID` — `uint64`

**Value**: the serialized span, encoded as either:
- Protobuf (`proto.Marshal`, encoding type `0x02`) — the default
- JSON (`json.Marshal`, encoding type `0x01`) — available as an alternative

**`UserMeta` byte**: stores the encoding type in the lower 4 bits. Reads use `item.UserMeta() & 0x0F` to determine how to deserialize the value.

**TTL**: all entries expire after the configured span TTL (default 72 hours). The expiry is set as `uint64(time.Now().Add(ttl).Unix())` (seconds since Unix epoch) in the Badger entry's `ExpiresAt` field.

**Sorting behavior**: because all three components after the prefix byte are encoded in big-endian, all spans belonging to the same trace cluster together, and within a trace they are sorted by start time and then span ID. This allows `GetTrace` to retrieve all spans of a trace via a single prefix scan without any additional filtering.

---

### 2. Service Name Index (`0x81`)

An index entry is written for each span, keyed by the service name of the span's process.

**Key** (variable size):
```
[0x81][serviceName: variable][startTime: 8B][traceID.High: 8B][traceID.Low: 8B]
```

- `serviceName` — UTF-8 bytes of the service name, no length prefix or separator
- `startTime` — `uint64`, microseconds since Unix epoch
- `traceID` — the 16-byte trace ID (High then Low, big-endian)

**Value**: empty (`nil`)

**Purpose**: enables scanning all trace IDs associated with a given service within a time range. The reader seeks to `[0x81][serviceName]` and iterates in reverse (latest first), extracting the trailing 16 bytes of each key as the trace ID.

**TTL**: same as the corresponding primary span record.

---

### 3. Operation Name Index (`0x82`)

An index entry is written for each span, keyed by the concatenation of service name and operation name.

**Key** (variable size):
```
[0x82][serviceName + operationName: variable][startTime: 8B][traceID.High: 8B][traceID.Low: 8B]
```

- `serviceName + operationName` — the two strings concatenated directly, no separator

**Value**: empty (`nil`)

**Purpose**: enables finding trace IDs for a specific service + operation pair within a time range.

**Note**: because the service name and operation name are concatenated without a separator, a service named `"foo"` with operation `"bar"` produces the same prefix as a service named `"foobar"` with operation `""`. The reader guards against this ambiguity by checking that the full key prefix (up to the timestamp) matches exactly.

**TTL**: same as the corresponding primary span record.

---

### 4. Tag Index (`0x83`)

For each searchable tag key-value pair associated with a span, a separate index entry is written. Tags are indexed from three sources: `span.Tags`, `span.Process.Tags`, and `log.Fields` for each log entry.

**Key** (variable size):
```
[0x83][serviceName + tagKey + tagValue: variable][startTime: 8B][traceID.High: 8B][traceID.Low: 8B]
```

- `serviceName + tagKey + tagValue` — all three strings concatenated directly, no separators
- Tag values are converted to their string representation via `kv.AsString()` before being embedded in the key

**Value**: empty (`nil`)

**Purpose**: enables finding trace IDs for spans that carry a specific tag key-value pair within a given service.

**TTL**: same as the corresponding primary span record.

---

### 5. Duration Index (`0x84`)

One index entry is written per span, encoding the span's duration.

**Key** (variable size, fixed numeric portion):
```
[0x84][duration: 8B][startTime: 8B][traceID.High: 8B][traceID.Low: 8B]
```

- `duration` — `uint64`, span duration in microseconds (via `model.DurationAsMicroseconds`)
- `startTime` — `uint64`, microseconds since Unix epoch
- `traceID` — 16-byte trace ID

**Value**: empty (`nil`)

**Purpose**: enables range scans over span duration. A duration query scans forward from `[0x84][minDuration]` to `[0x84][maxDuration]`, collecting trace IDs. The result is used as a hash-set filter (`hashOuter`) that is intersected with results from other indexes before final trace retrieval.

**Key design rationale**: by placing `duration` before `startTime`, all keys for a given duration value are contiguous in the sorted keyspace, making range scans efficient. The time range filter is applied as a secondary check during the scan.

**TTL**: same as the corresponding primary span record.

---

### 6. Sampling Throughput Record (`0x08`)

Written by the adaptive sampling component to record observed request throughput.

**Key** (16 bytes allocated, 9 bytes used):
```
[0x08][startTime: 8B][0x00 × 7]
```

- `startTime` — `uint64`, microseconds since Unix epoch
- The remaining 7 bytes of the 16-byte allocation are implicitly zero

**Value**: JSON-encoded `[]*model.Throughput`

**No TTL**: sampling entries do not have an explicit expiry set via Badger's `ExpiresAt`. Cleanup relies on explicit deletion or Badger's value-log GC.

---

### 7. Sampling Probabilities/QPS Record (`0x09`)

Written by the adaptive sampling component to record computed sampling probabilities and QPS estimates.

**Key** (16 bytes allocated, 9 bytes used):
```
[0x09][startTime: 8B][0x00 × 7]
```

**Value**: JSON-encoded `ProbabilitiesAndQPS` struct:
```go
type ProbabilitiesAndQPS struct {
    Hostname      string
    Probabilities model.ServiceOperationProbabilities  // map[service]map[operation]float64
    QPS           model.ServiceOperationQPS             // map[service]map[operation]float64
}
```

**No TTL**: same as throughput records.

---

## Query Execution

The reader builds an *execution plan* that describes how to combine index results:

1. **Duration filter** (if present): scanned via `scanRangeIndex` using the duration index. Results are stored in `plan.hashOuter` (a set of trace IDs) for subsequent intersection.

2. **Index seeks** (service, operation, tags): for each index key prefix derived from the query parameters, `scanIndexKeys` iterates in reverse order (latest first) within the time range, extracting the trailing 16 bytes of each matching key as a trace ID.

   - When multiple index seeks are needed (e.g., tag + operation), all but the last are scanned first and their results are combined via `mergeJoinIds` (a sorted merge intersection). The final seek then filters against this merged set.

3. **Full table scan** (fallback, when no service name is specified): `scanTimeRange` iterates all primary span keys (`0x80` prefix) within the time range, sorted descending by start time.

4. **Trace hydration**: the resulting trace ID list is resolved to full `*model.Trace` objects by prefix-scanning primary span records for each trace ID.

---

## Service and Operation Discovery

Service names and operation names are not queried directly from Badger on every request. Instead, they are maintained in an **in-memory cache** (`CacheStore`) that mirrors the TTL semantics of the underlying index entries:

- `services: map[string]uint64` — maps service name to its expiry time (Unix seconds)
- `operations: map[string]map[string]uint64` — maps service → operation → expiry time

The cache is populated in two ways:
1. **On write**: `cache.Update(serviceName, operationName, expireTime)` is called after every `WriteSpan`, keeping the cache current without re-scanning Badger.
2. **On startup** (if `prefillCache=true`): the reader scans all service name index keys (`0x81`) and operation name index keys (`0x82`) to preload any entries persisted from a previous run.

Expired entries are lazily removed from the cache when `GetServices` or `GetOperations` is called.

---

## Storage Configuration

Key operational parameters (from [`config.go`](../../internal/storage/v1/badger/config.go)):

| Parameter              | Default          | Notes                                       |
|------------------------|------------------|---------------------------------------------|
| `TTL.Spans`            | 72 hours         | Expiry for all span-related Badger entries  |
| `Ephemeral`            | `true`           | Uses a temp directory; data lost on restart |
| `SyncWrites`           | `false`          | Async writes for performance                |
| `MaintenanceInterval`  | 5 minutes        | Frequency of value-log GC runs              |
| `MetricsUpdateInterval`| 10 seconds       | Frequency of metric collection              |
| `Directories.Keys`     | `<exe>/data/keys`| Directory for LSM key index (SSD preferred) |
| `Directories.Values`   | `<exe>/data/values` | Directory for value log (HDD acceptable) |

The separation of key and value directories allows placing the key index on faster SSD storage while the value log (which is written sequentially) can reside on slower spinning disk.

---

## Decision

The record layouts described above were chosen to satisfy the following requirements:

1. **Lexicographic range scans**: all time-based queries rely on big-endian encoding of timestamps so that key iteration maps directly to time-range iteration.
2. **Single-instance simplicity**: all record types share one Badger database, distinguished by prefix byte. This avoids the complexity of managing multiple database handles.
3. **Index-only secondary records**: secondary index keys carry no values (nil), keeping the index footprint minimal. The full span data is always fetched from the primary record after the index identifies the relevant trace IDs.
4. **TTL-driven expiry**: Badger's native per-entry TTL mechanism is used for automatic data expiration, eliminating the need for a background deletion job.
5. **Embedded operation**: Badger requires no external process, making the Badger backend suitable for single-binary, all-in-one deployments.

## Consequences

### Positive

- Simple deployment: no external storage infrastructure required.
- Automatic expiry via native Badger TTL.
- Efficient prefix and range scans over sorted keys.
- Duration queries can be intersected with other query criteria (contrast with Cassandra; see [ADR-001](001-cassandra-find-traces-duration.md)).

### Negative / Limitations

- **Not distributed**: Badger is a single-node store. It is not suitable for high-throughput or multi-instance deployments.
- **No spanKind in operations**: the operation name index does not encode span kind, so `GetOperations` returns operations without span kind information (tracked in [issue #1922](https://github.com/jaegertracing/jaeger/issues/1922)).
- **String concatenation without separators**: the absence of separators between service name, tag key, and tag value in composite index keys means that a suffix of one component can collide with a prefix of the next. The implementation handles this with exact-prefix length checks but it is a latent source of subtle bugs if the key format is extended.
- **No dependency index**: the dependency store computes dependency links via a full trace scan on every request rather than maintaining a dedicated index, which may be slow for large datasets.
- **Sampling entries have no TTL**: throughput and probabilities records are not automatically expired and accumulate indefinitely unless explicitly pruned.
- **Ephemeral by default**: the default configuration (`Ephemeral: true`) stores data in a temporary directory that is deleted on process exit, which may surprise users who expect data to persist across restarts.

## References

- [`internal/storage/v1/badger/spanstore/writer.go`](../../internal/storage/v1/badger/spanstore/writer.go) — `createTraceKV`, `createIndexKey`, `WriteSpan`
- [`internal/storage/v1/badger/spanstore/reader.go`](../../internal/storage/v1/badger/spanstore/reader.go) — `FindTraceIDs`, `scanIndexKeys`, `scanRangeIndex`, `scanTimeRange`, `getTraces`
- [`internal/storage/v1/badger/spanstore/cache.go`](../../internal/storage/v1/badger/spanstore/cache.go) — `CacheStore`
- [`internal/storage/v1/badger/samplingstore/storage.go`](../../internal/storage/v1/badger/samplingstore/storage.go) — `createThroughputKV`, `createProbabilitiesKV`
- [`internal/storage/v1/badger/config.go`](../../internal/storage/v1/badger/config.go) — `Config`, `DefaultConfig`
- [Badger documentation](https://dgraph.io/docs/badger/)
- [ADR-001: Cassandra FindTraceIDs Duration Query Behavior](001-cassandra-find-traces-duration.md)
