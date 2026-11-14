# RFC 0020: Badger Span Kind Index

- **Status:** Implemented in [#9972](https://github.com/jaegertracing/jaeger/pull/9972)
- **Author:** Atip Kajitamkul
- **Created:** 2026-04-18
- **Last Updated:** 2026-10-30
- **Related:** [#1922 Query service to support spanKind when retrieve operations](https://github.com/jaegertracing/jaeger/issues/1922) [#6376](https://github.com/jaegertracing/jaeger/pull/6376) [ADR-005 (Badger storage's record layouts)](../adr/005-badger-storage-record-layouts.md)

---

## Abstract

Badger's `GetOperations` does not currently support returning or filtering operations by span kind ([#1922](https://github.com/jaegertracing/jaeger/issues/1922)). As documented in [ADR-005 (Badger storage's record layouts)](../adr/005-badger-storage-record-layouts.md), the writer creates an operation-index entry (`0x82`) for each span. The key concatenates the service name, operation name, start time, and trace ID, while the value is empty (`nil`). Readers extract trace IDs from these keys for trace search and use the same index to rebuild the operation cache.

Because neither the index nor the cache retains span kind, supporting this feature requires changes to both. The existing key format also lacks a delimiter between the service and operation names. Changing the key layout affects how readers locate records. To keep existing data readable, existing entries must either be rewritten or remain accessible through support for both formats.

This RFC proposes a design for supporting span kind in Badger's `GetOperations`, informed by the design discussion and preload benchmark. The proposed design extends the existing `0x82` record's value and the operation cache. It preserves the current trace-search path, while the benchmark shows modest additional preload overhead in the measured dataset. The RFC explains this choice, compares the alternatives, and describes the compatibility requirements for rollout and rollback.

---

## 1. Motivation

The same operation name can occur with different span kinds within a service. Identifying operations by name alone loses this distinction. `GetOperations` should return each distinct `(name, kind)` pair and allow callers to filter the results by span kind.

These distinctions need to survive a restart. The cache receives operation information during ingestion, but startup prefill rebuilds it from the operation index. Updating the cache alone would leave it unable to recover span kinds from stored records, so the index must retain that information as well.

The index also supports `FindTraceIDs`, which searches by service and operation within a time range. Placing kind between service and operation would split those records across different prefixes, potentially requiring multiple seeks or a broader scan when no kind is specified. Since `GetOperations` reads from the cache, the disk layout should support rebuilding that cache while keeping trace searches efficient. The change must also preserve access to existing data during upgrades and rollback.

---

## 2. Current state

The [writer](../../internal/storage/v1/badger/spanstore/writer.go) creates an operation-index entry for each span using this format:

```text
Key:
[0x82:1B][serviceName + operationName:variable]
[startTime:8B][traceID.High:8B][traceID.Low:8B]

Value: nil
```

The key occupies `25 + len(serviceName) + len(operationName)` bytes. The timestamp and trace-ID components use big-endian encoding. `startTime` is measured in microseconds since the Unix epoch; expiry is stored separately in `ExpiresAt`, in seconds, and is calculated from the write time plus the configured TTL.

Service and operation names are concatenated without a separator. For example, `("ab", "cd")` and `("a", "bcd")` produce identical combined bytes. Checking the length or contents of that combined prefix cannot recover the missing field boundary. A service-prefix scan can also match longer service names that begin with the same bytes.

The [cache](../../internal/storage/v1/badger/spanstore/cache.go) currently stores `service → operation name → expiry`. Ingestion updates it directly. When startup prefill is enabled, the reader scans the service index (`0x81`) and operation index (`0x82`) to rebuild it.

The [reader's `GetOperations`](../../internal/storage/v1/badger/spanstore/reader.go) passes only the service name to the cache and returns operations with an empty `SpanKind`. It does not apply the query's kind filter. `FindTraceIDs`, meanwhile, constructs its service-and-operation prefix from the query and scans index keys with value prefetching disabled. It does not resolve kinds through the operation cache.

---

## 3. Requirements

The implementation should:

- Return distinct `(name, kind)` pairs within each service and support filtering by kind.
- Rebuild the cache from unexpired index entries after a restart, preserving all recorded kinds and following the expiry rules defined in this RFC.
- Specify the key and value formats, including field widths, string lengths, and how absent kinds, unrecognized kinds, and legacy values are represented.
- Keep traces searchable by service and operation during upgrades, gate changes, and supported rollback by continuing to write `0x82` entries that older binaries can read.
- Use the new `GetOperations` behavior only when the gate is enabled and the database has passed its readiness check. Otherwise, keep the existing behavior. Primary and archive databases must be checked separately.

The change is limited to operation discovery. Backfilling historical kind information, changing trace-search filters, and separating service and operation names in the key are outside its scope. Alternative key formats are discussed in [Section 4.4](#44-alternatives).

---

## 4. Proposed design

This RFC proposes keeping the existing `0x82` key and storing span kinds in its value, following [Yuri’s suggestion](https://github.com/jaegertracing/jaeger/pull/8402#discussion_r3120670715). The operation cache would read these values at startup so `GetOperations` can return and filter operations by kind.

### 4.1 Record layout and encoding

The key would remain unchanged. The value would start with a version byte and a flags byte, followed by one or more kinds:

```text
Key:
[0x82:1B][serviceName + operationName:variable]
[startTime:8B][traceID.High:8B][traceID.Low:8B]

Value:
[version:1B = 0x01][flags:1B]
repeated until the end of the value:
    [kindLength:4B][spanKind:kindLength bytes]
```

The key would use `25 + len(serviceName) + len(operationName)` bytes, and the value would use `2 + Σ(4 + kindLength)` bytes. Integer fields shown above would use unsigned big-endian encoding. Start time would remain in Unix microseconds.

The legacy bit, `0x01`, would mean that some older spans represented by the entry have no stored kind information. The writer would set this bit when merging with an existing `nil` value and keep it on later writes. All other flag bits would be reserved and set to zero.

Kind would come from the first span-level `span.kind` tag. Strings would be stored unchanged, including unrecognized kind strings. Other value types would be converted with `KeyValue.AsString()`, as they are in the existing tag index. An absent or empty tag would be stored as a zero-length kind and returned as `""`. Each kind would appear once, sorted by its encoded bytes.

Expiry would stay in Badger’s `ExpiresAt` field, in Unix seconds, and apply to the entire entry. A merge would keep the later expiry. Zero would mean no expiry and take precedence over any finite expiry.

A `nil` value would mean that kind information was not recorded, while an encoded empty kind would represent an absent or empty tag. During prefill, the database would be marked as not ready if an entry has a `nil` value, the legacy bit set, an unsupported version, unknown flag bits, or an invalid value.

### 4.2 Collision handling

The `0x82` key does not include span ID or kind. Two spans can therefore have the same index key but different kinds. As discussed in [#6376](https://github.com/jaegertracing/jaeger/pull/6376#discussion_r1900318241), storing only one kind would let a later write overwrite the earlier kind.

The writer would read the existing `0x82` and `0x81` entries in the same transaction that writes the span. It would add the span’s kind to the operation entry without removing existing kinds, then keep the later of the service entry’s expiry and the merged operation entry’s expiry. A new service entry would use the operation entry’s expiry, with zero taking precedence as no expiry. This would keep the service available for prefill while the operation entry remains unexpired. On `ErrConflict`, the writer would start a new transaction, read both entries again, and retry. After a successful commit, it would update the caches with all kinds from the merged entry and the resulting expiries, without shortening any expiry already cached.

TTL is counted from write time, so spans sharing a key may expire at different times. With one expiry for the whole set, `GetOperations` could still return a kind after all spans carrying it for that operation have expired.

Repeated writes to an entry with the legacy bit set could also keep extending its expiry, leaving the database unable to become ready. The readiness check would finish before writers start, as described in [Section 7.3](#73-database-warm-up-and-expiry).

### 4.3 Rationale

`GetOperations` reads from the cache, so it does not need kind in the index key to filter its results. Storing kind in the value would let the cache load this information at startup while `FindTraceIDs(service, operation)` continues to scan the same keys. This would avoid adding another index and supporting both key formats during upgrade and rollback.

The [benchmark](https://gist.github.com/Audio431/5dda84ae8189c60932d9e1156003204c), described in [section 6](#6-preload-benchmark), measured about 26 ms of additional operation prefill time per generated service and 90 ms per prefill pass on about 5 GB of data. These results support reading values during prefill for that dataset. They do not compare `0x82` with `0x85`, or measure the cost of decoding the kind set and performing the additional reads and retries when writing spans.

### 4.4 Alternatives

The [writer](https://github.com/jaegertracing/jaeger/blob/main/internal/storage/v1/badger/spanstore/writer.go#L120) calculates span index prefixes as `(prefix & 0x0F) | 0x80`. Since `0x80`–`0x84` are already used, a new index could use `0x85`–`0x8F`. A `0x85` format could separate service and operation names and include kind in the key. However, putting kind between those names would require operation trace searches to scan a separate prefix for each kind.

Putting a one-byte kind between the timestamp and trace ID would keep the trace ID in the last 16 bytes, but move the start of the timestamp from 24 to 25 bytes before the end of the key. The [reader currently assumes 24 bytes](https://github.com/jaegertracing/jaeger/blob/main/internal/storage/v1/badger/spanstore/reader.go#L565), so that layout would also require changes to the existing scans. Keeping the key unchanged avoids these changes.

Separators could fix the ambiguity between service and operation names, provided the format also defines how to escape separator bytes within those names. That key-format change is outside the scope of this RFC.

---

## 5. Cache and operation queries

The cache would identify operations by both name and kind within a service:

```text
service → tracestore.Operation{Name, SpanKind} → expiry
```

Each pair would use the latest expiry among the index entries containing it, with zero treated as no expiry. After a successful write, the cache would be updated with all kinds and the expiry from the merged entry, not just the new span’s kind. Prefill would use the same rules when reading stored entries.

When the gate is enabled and the database is ready, `GetOperations` would return all distinct pairs if the kind filter is empty, or only pairs matching the requested kind otherwise. An explicitly stored empty kind would be returned as `""`. Results would be sorted by name and then kind.

When the gate is disabled or the database is not ready, `GetOperations` would keep its current behavior: return each operation name once, with `SpanKind` set to `""`, without applying the kind filter. Operation names from legacy entries would remain available for this fallback. Their `nil` values would not be interpreted as evidence that the original spans had an empty kind. The primary and archive databases would each have their own readiness state.

The writer would extract kinds from spans as described in [Section 4.1](#41-record-layout-and-encoding), and prefill would decode them from the stored values. Prefill would report decoding errors and entries with incomplete kind information, preventing the database from being marked ready with an incomplete cache.

Prefill must read every kind stored in each entry it processes. Reading one entry for an operation is not enough to recover all its kinds or their latest expiries, because other entries may contain additional kinds or expire later. Any optimization that skips entries must preserve both.

---

## 6. Preload benchmark

At Yuri’s request, a [benchmark](https://gist.github.com/Audio431/5dda84ae8189c60932d9e1156003204c) compared prefill with and without `ValueCopy(nil)` on approximately 5 GB of tracegen data. The dataset contained 499 SST tables and six services: five generated services and Jaeger’s internal service. Both versions read the same operation records containing a one-byte dummy value, `0x01`. The difference was whether the reader copied that value.

Both versions used `badger.DefaultIteratorOptions`, with `PrefetchValues: true`. The values were stored inline in the LSM, so the baseline already included value prefetching. The test measured the extra cost of calling `ValueCopy(nil)` during prefill.

The following figures use runs 2–3. The first run with `ValueCopy` was treated as a cold-cache outlier.

| Measurement | Without `ValueCopy` | With `ValueCopy` | Difference |
| --- | --- | --- | --- |
| Service prefill | ~2.39 s | ~2.35 s | Run-to-run variation |
| Operation prefill, average per generated service | ~499 ms | ~525 ms | +~26 ms (~5%) |
| Total prefill per pass | ~4.88 s | ~4.97 s | +~90 ms (~2%) |

The test did not measure copying and allocation costs separately. At the time of the measurement, the factory created two readers with prefill enabled, so the totals described one prefill pass rather than the full startup. The current factory retains one prefilled reader.

These percentages apply to this dataset and these runs. They do not show that the overhead would stay at 5% on larger databases. `preloadOperations` seeks to a service prefix and scans every matching record. As the dataset grows, the cost can change with the amount of cached data, disk access, and garbage collection.

The test used a dummy byte rather than the proposed kind-set encoding, and the number of distinct kinds in the generated data was not checked. Further measurements should cover decoding the set, cache memory use with realistic operation names and kinds, and the additional transaction reads and retries when writing spans.

The results were [shared in the design discussion on 2026-04-30](https://github.com/jaegertracing/jaeger/pull/8402#discussion_r3169432632).

### Implementation microbenchmarks

`BenchmarkOperationIndex` measures the versioned encoding, cache reconstruction, and transactional merges on an Apple M4 with Go 1.27.0 and on-disk Badger with its default asynchronous writes. Three runs with `-benchtime=500ms -count=3` produced:

| Measurement | Time | Allocated bytes |
| --- | --- | --- |
| Sequential writes to an existing three-kind entry | 9.02–9.53 µs/write | ~5.92 KB/write |
| Concurrent writes contending on that entry, including retries | 12.81–13.65 µs/write | ~22.7–22.8 KB/write |
| Full prefill and readiness scan over 6,000 spans | 5.74–5.76 ms/pass | ~2.80 MB/pass |
| Cache construction for 3,000 operation/kind pairs | 0.257–0.273 ms/build | 388,376 bytes/build |

The prefill dataset contains 20 services, 100 HTTP-style operation names per service, and three kinds per operation. Cache construction reuses preallocated names and constant kind strings; its allocation count excludes those strings and is not a retained-heap measurement. These are warm, small-dataset absolute measurements, not a comparison against the old writer or a substitute for the earlier 5 GB dataset. The concurrent case includes retry overhead but does not count retries separately. Larger-database and durable-write measurements remain necessary before claiming production performance characteristics.

---

## 7. Rollout and rollback

The proposed gate, `jaeger.badger.operationIndex`, would control whether `GetOperations` returns and filters operations by kind. It would remain disabled by default while the readiness check is required.

The gate would apply to the process, but readiness would be tracked separately for each Badger database. The primary database could therefore serve results with kinds while the archive database continues returning names only.

### 7.1 Writes and queries

Upgraded writers would write the value format described in [Section 4.1](#41-record-layout-and-encoding) whether the gate is enabled or disabled. The `0x82` key would remain unchanged.

| Gate | Database readiness | `GetOperations` | `FindTraceIDs` for service and operation |
| --- | --- | --- | --- |
| Off | Any | Distinct names, empty kinds, no kind filtering | Existing `0x82` key scan |
| On | Not ready | Distinct names, empty kinds, no kind filtering | Existing `0x82` key scan |
| On | Ready | Distinct name/kind pairs with optional kind filtering | Existing `0x82` key scan |

Backfill is outside the scope of this RFC, so entries with missing historical kind information must expire before the database can become ready. A write to an existing legacy `nil` entry would set the legacy bit, and later writes would preserve it. Adding new kinds would not recover the missing kind information for earlier spans.

### 7.2 Disabling the gate and rolling back

Disabling the gate would restore the existing `GetOperations` behavior without removing stored kind information. Upgraded writers would continue writing the new values.

An older writer would resume writing `nil` values and could overwrite kind information in entries with matching keys. After another upgrade, the database would need to pass the readiness check again before serving results with kinds.

The implementation must verify rollback against a supported older binary, including reading traces and searching by service and operation after the upgraded writer has stored the new values.

### 7.3 Database warm-up and expiry

Each database would start with readiness set to false. Before accepting span writes, startup would:

1. Complete the service and operation cache prefill.
2. Scan all unexpired `0x82` entries and count `nil` values, entries with the legacy bit set, and entries whose values cannot be decoded under the rules in Section 4.1.
3. Make the completed cache available and mark the database ready only if prefill and the scan succeed and all three counts are zero.

Startup logs would report the result and counts for each database. Read errors would be reported and would never count as a successful check.

The check would finish before writers start, preventing an unfinished write from restoring an entry with incomplete kind information after the database is marked ready. If coverage is incomplete, that database would continue returning names only for the rest of the process lifetime. Upgraded writers would still record kinds for new spans, and the next restart would run the check again.

The check would use the entries actually present in the database, rather than assuming that a release-stage duration or the current TTL is sufficient. Older entries may have longer expiry times. Repeated writes to an entry with the legacy bit set may also extend its expiry, as described in Section 4.2.

Readiness would not be saved and reused across restarts. After a binary rollback or backup restore, the database would have to pass a new check before serving results with kinds.

---

## 8. Consequences

Storing kinds in the index and identifying cached operations by `(name, kind)` would allow the cache to rebuild multiple kinds of the same operation after a restart. Index values would be larger, and the cache could hold more entries. Writing spans would also require reading the existing value, with transaction retries when concurrent writes conflict. These costs are not covered by the preload benchmark.

Keeping the `0x82` key unchanged would preserve the existing operation trace-search path, including searches over legacy entries. The ambiguity between concatenated service and operation names would remain.

Using one expiry for all kinds in an entry could keep a kind discoverable after all spans carrying it have expired. The same expiry would apply to the legacy bit, so repeated writes to an entry with that bit set could prevent the database from becoming ready indefinitely.

Kinds missing from legacy entries could not be recovered from the operation index alone. Since backfill is outside the scope of this RFC, those entries would need to expire before the database could serve results with kinds. Until then, `GetOperations` would continue returning names only, without applying the kind filter.

The implementation PR must update [ADR-005 (Badger storage’s record layouts)](../adr/005-badger-storage-record-layouts.md) to describe the new `0x82` value format, collision handling, expiry rules for `0x81` and `0x82`, and cache changes. It must also update the statement that secondary index values are always `nil`, because new `0x82` entries would store span kinds. The ADR should clarify that checking the full concatenated prefix cannot distinguish different combinations of service and operation names.

---

## 9. Implementation plan

The value codec, transactional merges, cache, prefill, gate, and per-database startup checks are implemented in [#9972](https://github.com/jaegertracing/jaeger/pull/9972). Regression tests cover collisions, concurrent writes, shorter and longer TTLs across disk reopen, non-expiring entries, legacy expiry, backup restore, malformed values, and failed writes. Both direct Badger integration and the Jaeger server end-to-end corpus pass with span-kind checks enabled. ADR-005 describes the resulting layouts and limitations.

A cross-revision storage test using separate binaries built from the pre-fix revision `e5274265` and the changed source verifies new-write/old-read compatibility, service/operation trace searches, an old writer overwriting an entry, and fallback after re-upgrade. The server-level `TestBadgerStorageRollback` also passes against the official Jaeger v2.22.0 release: it checks new writes, a new-version restart, disabling the gate, rollback with an old writer overwriting a shared entry, and re-upgrade. Trace hydration, service/operation searches, kind filtering, and fallback are checked through the query API in each phase. The release archive's SHA-256 was verified against GitHub's published asset digest. Larger-dataset performance validation and review remain outstanding; delivering PR references can be recorded when the implementation is submitted.

Native Windows Server 2022 AMD64 validation with Go 1.27.0 also passes the spanstore unit suite with the race detector, 50 repeated concurrent-write regressions, and shared trace-storage assertions including operation discovery. The broader Windows commands are not fully green: factory tests `TestInitializationErrors` and `TestForCodecov`, and sampling integration `TestBadgerStorage/GetThroughput`, fail identically on unchanged revision `e5274265`. No assertion was weakened to bypass them. Full-server release rollback was verified on macOS, not Windows.

- ✅ **Implement the value format and collision handling.** The existing `0x82` key retains the encoding described in [Section 4.1](#41-record-layout-and-encoding), including legacy values, shared expiry, and retries on `ErrConflict`. Regression tests cover configured-TTL changes, commit order, restart behavior, and concurrent writes to the same `0x82` key.

- ✅ **Update the cache, prefill, and `GetOperations`.** Operations are stored by name and kind within each service. Cache updates and prefill preserve every kind and expiry from committed entries. Tests cover absent, empty, unknown, non-string, and control-byte kinds, multiple kinds, restarts, and decoding errors.

- ✅ **Add the gate and database readiness checks.** The alpha gate and per-database readiness state implement the behavior in [Section 7](#7-rollout-and-rollback). Tests cover both gate states, legacy expiry, backup restore, and separate old/new binary rollback.

- ✅ **Enable the integration tests for span kind.** Badger integration tests run with the gate enabled and a ready database; `getOperationsMissingSpanKind` was removed from `capabilities.Badger()`. Gate-disabled and not-ready fallbacks have separate unit coverage.

- ✅ **Measure the implementation and update the documentation.** `BenchmarkOperationIndex` measures value encoding, cache reconstruction, and transactional merges. ADR-005 records the `0x82` value format, collision handling, expiry rules, and cache changes.

---

## 10. References

1. [#1922: span kind support in operation queries](https://github.com/jaegertracing/jaeger/issues/1922).
2. [#6376: earlier implementation and dual-lookup discussion](https://github.com/jaegertracing/jaeger/pull/6376).
3. [Yuri's review of the key layout and value-based alternative](https://github.com/jaegertracing/jaeger/pull/8402#discussion_r3120670715).
4. [Badger preload benchmark](https://gist.github.com/Audio431/5dda84ae8189c60932d9e1156003204c).
5. [Benchmark results posted to the design discussion](https://github.com/jaegertracing/jaeger/pull/8402#discussion_r3169432632).
6. [Review covering document scope, encoding, and rollout](https://github.com/jaegertracing/jaeger/pull/8402#pullrequestreview-5644968984).
7. [ADR-005 (Badger storage's record layouts)](../adr/005-badger-storage-record-layouts.md).
8. [Badger writer](../../internal/storage/v1/badger/spanstore/writer.go), [reader](../../internal/storage/v1/badger/spanstore/reader.go), and [cache](../../internal/storage/v1/badger/spanstore/cache.go).
9. [Storage integration capabilities](../../internal/storage/integration/capabilities/capabilities.go).
