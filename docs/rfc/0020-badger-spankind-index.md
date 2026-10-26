# RFC 0020: Badger Span Kind Index

- **Status:** Proposed
- **Author:** Atip Kajitamkul
- **Created:** 2026-04-18
- **Last Updated:** 2026-10-26
- **Related:** [#1922 Query service to support spanKind when retrieve operations](https://github.com/jaegertracing/jaeger/issues/1922) [#6376](https://github.com/jaegertracing/jaeger/pull/6376) [ADR-005 (Badger storage's record layouts)](../adr/005-badger-storage-record-layouts.md)

---

## Abstract

Badger's `GetOperations` does not currently support returning or filtering operations by span kind ([#1922](https://github.com/jaegertracing/jaeger/issues/1922)). As documented in [ADR-005 (Badger storage's record layouts)](../adr/005-badger-storage-record-layouts.md), the writer creates an operation-index entry (`0x82`) for each span. The key concatenates the service name, operation name, start time, and trace ID, while the value is empty (`nil`). Readers extract trace IDs from these keys for trace search and use the same index to rebuild the operation cache.

Because neither the index nor the cache retains span kind, supporting this feature requires changes to both. The existing key format also lacks a delimiter between the service and operation names. Changing the key layout affects how readers locate records. To keep existing data readable, existing entries must either be rewritten or remain accessible through support for both formats.

This RFC proposes extending the operation index and cache to support span kind in `GetOperations`. It discusses alternative record layouts and their trade-offs, and sets out the compatibility requirements for rollout and rollback.

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

- Return distinct operation-name and span-kind pairs and apply an optional kind filter.
- Preserve those pairs and their expiry when rebuilding the cache after a restart.
- Define the complete record format, including field widths, string boundaries, and the representation of absent and unrecognized kinds.
- Keep existing traces searchable by operation during upgrade, gate changes, and supported binary rollback.
- Require warm-up of each database before enabling the feature, with an explicit check of data coverage.
- Retain legacy writes throughout every stage that supports rollback to a binary that reads only the legacy index.

The change is intended to support operation discovery. Changes to trace-search filtering semantics and backfilling historical kind information are outside its initial scope. Fixing the service/operation boundary would require a key-format change and is considered separately below.

---

## 4. Record layout alternatives

### 4.1 Store kind in the existing `0x82` value

[Yuri suggested adding span kind to the value of the existing record](https://github.com/jaegertracing/jaeger/pull/8402#discussion_r3120670715). This would allow prefill to recover kind while preserving the key used by trace search.

A possible encoding would retain the existing key and add a versioned string value:

```text
Key:
[0x82:1B][serviceName + operationName:variable]
[startTime:8B][traceID.High:8B][traceID.Low:8B]

Value:
[version:1B = 0x01][spanKind:K bytes]
```

Under this encoding, `spanKind` would contain the bytes of the `span.kind` string tag. The value would occupy `1 + K` bytes, with the end of the value marking the end of the string. The version byte would distinguish a new record with an absent kind from an older record whose value is empty.

| Input or record | Value | Kind returned by `GetOperations` |
| --- | --- | --- |
| Recognized string, such as `server` | `0x01` followed by the original string bytes | Original string |
| Unrecognized non-empty string | `0x01` followed by the original string bytes | Original string |
| Absent tag or empty string | `0x01` alone | `""` |
| Legacy empty value | Unchanged empty value | `""` |

This would preserve unknown strings rather than treating the input as a closed OTel enum from 0 to 5. An empty query filter would mean no kind filter; a non-empty filter would match the stored string exactly. Malformed values and unsupported versions would need explicit error handling during prefill. The handling of non-string `span.kind` tags remains an encoding question to resolve against the model's conversion rules.

The existing trace-search reader ignores values, so populating them would preserve its search path. Prefill would need to read and decode values, and the concatenation ambiguity in the existing key would remain. The benchmark in section 6 measures part of that additional prefill work.

The cache collision concern is separate from the placement of kind on disk. A cache keyed only by operation name would collapse different kinds of that name. An operation-index key also contains start time and trace ID, so sharing an operation name does not by itself cause one disk record to overwrite another. A narrower case remains when two spans share the entire index key, which omits span ID; that case needs a policy before a single-kind value can guarantee complete reconstruction of the cache.

### 4.2 Store kind in a new key

The original proposal placed a one-byte kind between service and operation under a new `0x85` prefix. A new prefix would let readers distinguish an updated key format from the existing one without rewriting old entries. This explains why a key change could use a new prefix; it does not establish that kind must be placed in the key.

A fixed-width kind byte would have a known offset only after the exact service boundary was known. A prefix match alone cannot establish that boundary: longer service names can share the prefix, and names can contain bytes that coincide with kind encodings. The kind byte cannot serve as a separator.

Putting kind between service and operation would favor scans by service and kind. `GetOperations` does not perform those scans on each request: it reads from the cache. The disk query is `FindTraceIDs(service, operation)`, and placing kind before operation would require multiple seeks or a broader scan to find records of all kinds. Cache-based kind resolution would also need to establish that the cache contains every kind relevant to the requested time range.

[Manik's earlier PR, #6376](https://github.com/jaegertracing/jaeger/pull/6376), used a different ordering in its final diff:

```text
Key:
[0x85:1B][serviceName + operationName:variable]
[startTime:8B][kind:1B][traceID.High:8B][traceID.Low:8B]

Value: nil
```

[Yuri suggested putting kind after the timestamp](https://github.com/jaegertracing/jaeger/pull/6376#issuecomment-2568123111) so that a query could reach the service, operation, and time range without knowing kind. The PR included dual lookup of old and new indexes and was closed without merging.

Placing kind after the timestamp would avoid partitioning the time range by kind, but would still require updated key parsing and a compatibility transition. A one-byte encoding would also need a complete mapping from tag strings, including absent and unrecognized values. The mapping cannot be inferred from OTel enum numbers, and the encoding in section 4.1 would not automatically apply to a byte embedded in the key.

### 4.3 Separate variable-length fields

A new key format could also remove the ambiguity between service and operation. For example, a separated key with kind in the value could use:

```text
Key:
[0x85:1B][E(serviceName):variable][0x00:1B]
[E(operationName):variable][0x00:1B]
[startTime:8B][traceID.High:8B][traceID.Low:8B]

Value:
[version:1B = 0x01][spanKind:K bytes]
```

The key would occupy `27 + len(E(serviceName)) + len(E(operationName))` bytes. The value would use the string encoding described in section 4.1. The new prefix would need to be checked for availability when implemented.

Names can contain bytes `0x00`–`0x05`, so a literal `0x00` separator requires an escaping rule. One possible rule for `E` is:

| Input byte | Encoded bytes |
| --- | --- |
| `0x00` | `0x01 0x01` |
| `0x01` | `0x01 0x02` |
| Any other byte | Unchanged |

With that rule, an unescaped `0x00` would mark the end of a field. Empty names and names sharing a prefix would remain distinguishable. The final encoding would need tests for those cases and for embedded control bytes.

Adding separators under `0x82` would change the key expected by older readers. Their presence would not identify a new format reliably because old names can already contain `0x00`. Separating fields therefore needs a distinguishable key format and a compatibility plan, even when kind remains in the value.

### 4.4 Trade-offs

| Approach | Benefit | Cost or limitation |
| --- | --- | --- |
| Kind in the existing `0x82` value | Preserves the current operation-search key | Prefill must read values; existing field ambiguity remains |
| Kind between service and operation | Groups records by service and kind | Operation trace search must account for multiple kinds |
| Kind after timestamp | Preserves a contiguous service, operation, and time-range scan | Requires new parsing, a kind encoding, and support for old data |
| Separated fields with kind in the value | Gives service and operation explicit boundaries without putting kind in the search prefix | Requires a new key format and support for both indexes during transition |
| Kind only in the cache | Avoids a persistent-format change | Cannot recover kinds from the current index after restart |

The value-based approach offers the smaller change to trace search. A new key format offers a way to address field boundaries and, if kind forms part of record identity, distinguish records by kind. The benchmark helps compare prefill costs; field boundaries and record identity also need to be considered before settling the format.

---

## 5. Cache and operation queries

The cache would identify operations by both name and kind within a service:

```text
service → tracestore.Operation{Name, SpanKind} → expiry
```

This would preserve different kinds of the same operation name. Each pair would retain the latest expiry among its contributing records and expire independently of other pairs.

With the feature enabled, `GetOperations` would return all distinct pairs when the kind filter is empty and exact matches when it is non-empty. Missing kinds would be returned as `""`. Under the value-based approach, a legacy empty value would also decode to an empty kind because it contains no historical kind information. Results would be ordered by name and then kind.

With separate old and new indexes, the kind-aware cache would be rebuilt from the new index after coverage had been verified. Legacy entries would still support the name-only view while the gate was off. They must not add an artificial empty-kind pair for every record also represented in the new index. Combining trace IDs from both indexes and combining operation metadata are different operations.

Ingestion and prefill would use the same extraction and decoding rules. Cache updates would follow a successful storage write. Prefill would retain the stored expiry and report decoding failures so that an incomplete cache could not be treated as a successful warm-up.

Prefill must also retain every kind represented by the records it scans. Seeking directly to the next operation after reading one record could skip other kinds of that operation. Such an optimization would need to demonstrate that it preserves all distinct pairs.

---

## 6. Preload benchmark

At Yuri's request, a [benchmark published on 2026-04-30](https://gist.github.com/Audio431/5dda84ae8189c60932d9e1156003204c) compared prefill with and without `ValueCopy(nil)` on approximately 5 GB of tracegen data. The comparison used the same dataset, reported as 499 SST tables and six services: five generated services and Jaeger's internal service. Operation records contained a one-byte dummy value, `0x01`; the comparison changed whether the reader copied that value.

Both variants used `badger.DefaultIteratorOptions`, including `PrefetchValues: true`. The one-byte values were stored inline in the LSM. The baseline therefore already included value prefetching, and the experiment measured the additional work of retrieving and copying those values.

The gist reports the following figures from runs 2–3, treating the first value-copy run as a cold-cache outlier:

| Measurement | Without `ValueCopy` | With `ValueCopy` | Difference |
| --- | --- | --- | --- |
| Service prefill | ~2.39 s | ~2.35 s | Run-to-run variation |
| Operation prefill, average per generated service | ~499 ms | ~525 ms | +~26 ms (~5%) |
| Total prefill per pass | ~4.88 s | ~4.97 s | +~90 ms (~2%) |

The measured increase was small in this setup and is consistent with copying and allocation work on values that were already prefetched. Those costs were not profiled separately. The factory also created two readers with prefill enabled, so the total in the table describes one prefill pass rather than the full startup.

The result provides evidence for considering kind-in-value, but does not establish that the overhead will remain at 5% for larger databases. `preloadOperations` seeks to a service prefix and then visits every matching record. Copying a value adds work to that scan; the relative cost can change with cache residency, storage reads, and garbage collection as the dataset grows.

The experiment used a dummy byte rather than the final kind encoding, and kind diversity in the generated data was not verified. The implementation should measure decoding costs and cache growth with realistic name/kind pairs. The [benchmark results are available in the design discussion](https://github.com/jaegertracing/jaeger/pull/8402#discussion_r3169432632).

---

## 7. Rollout and rollback

A proposed feature gate, `jaeger.badger.operationIndex`, would control the new `GetOperations` behavior. Writers would prepare the database while the gate is off. Enabling it would require successful warm-up of that database, including a complete cache prefill and verified coverage of retained data. A release reaching Alpha or Beta would not establish that readiness.

The storage behavior would depend on the chosen record format.

### 7.1 Extending the existing value

If kind is added to the existing `0x82` value, upgraded writers would write the new value in either gate state. The key used by trace search would remain the same.

| Gate | Writes | `GetOperations` | `FindTraceIDs` for service and operation |
| --- | --- | --- | --- |
| Off | Existing `0x82` key with kind metadata in the value | Legacy behavior: distinct names, empty kinds, no kind filtering | Existing `0x82` key scan |
| On, after warm-up | Same key and value format | Distinct name/kind pairs with optional kind filtering | Same `0x82` key scan |

The reader would accept both legacy empty values and the new value encoding. While the gate is off, it could retain pairs internally and project them to the legacy result. Complete kind coverage would require the unexpired legacy empty-value records to have expired or acquired equivalent metadata before enabling the gate. A version marker, as in section 4.1, would distinguish those records from new records whose kind is intentionally absent.

Disabling the gate would not remove metadata already written. An older binary would still find traces using the unchanged keys and ignore the values, subject to verification against an actual older binary. Its writer would resume producing empty values, so enabling the feature after another upgrade would require a fresh readiness check.

### 7.2 Introducing a new key format

If the key changes, writers would write both the legacy `0x82` index and the new index while the gate is off and throughout every stage that supports binary rollback.

| Gate or transition | Writes | `GetOperations` | `FindTraceIDs` for service and operation |
| --- | --- | --- | --- |
| Off, warming up | Legacy and new indexes | Legacy behavior; prefill prepares the kind-aware cache from the new index | Legacy `0x82` scan |
| On, after warm-up | Legacy and new indexes | Distinct pairs from the new index with optional kind filtering | Deduplicated union of legacy and new index results |
| Disabled again | Legacy and new indexes | Legacy behavior | Legacy `0x82` scan |
| Binary rollback | Older writer continues legacy writes | Older binary's behavior | Legacy `0x82` scan, including records kept current by dual-write |

The union read would preserve ordering and apply limits after combining and deduplicating results. It would not rely on the cache to discover all kinds needed for trace search. Its new-index scan would follow the chosen key layout; a layout with kind before operation would still need kind fan-out or a broader scan.

Continuing legacy writes is necessary for rollback to an older binary. A union reader in the upgraded binary cannot protect a reader in an older binary that understands only `0x82`. Both index entries should be committed with the span and share its expiry so a failed write does not leave only one representation.

After the supported rollback window has closed, the implementation could stop legacy writes. Removing legacy reads would require a separate check that no retained record depends on them.

### 7.3 Database warm-up and expiry

Warm-up must establish coverage for the database being enabled. It should verify that compatible writers have populated the chosen representation, that retained records predating that coverage have expired or have a verified new representation, and that prefill has completed successfully. The implementation must expose enough readiness information to make this check enforceable before enabling the gate.

Every current `0x82` record receives an expiry of write time plus TTL. As long as writers keep creating legacy records, the `0x82` index does not drain. During dual-write warm-up, it is the records written before dual-write began that can expire; the legacy index as a whole remains populated.

A fixed release-stage duration or a wait based only on today's TTL is insufficient. The check must account for actual expiry times, including records written under a longer TTL. Restoring an older backup or writing with a legacy binary can invalidate previously established coverage and must trigger another readiness check.

Switching reads to only the new index before coverage is established would hide unexpired traces. Stopping legacy writes while binary rollback remains supported would make spans written during that period invisible to the older operation-search path. The rollout must avoid both gaps.

---

## 8. Consequences

Persisting kind and using `(name, kind)` as the cache identity would allow operation discovery to retain multiple kinds of the same name across restart. The cache could hold more entries, and prefill would need to process the additional metadata.

Adding kind to the existing value would preserve the trace-search key but retain its field-boundary ambiguity. A new key format could address that ambiguity and change record identity, at the cost of dual writes, support for multiple formats, and a longer compatibility transition. Either approach must define how it handles records with no kind metadata and repeated complete index keys.

Legacy entries would remain searchable by operation. Their kinds could not be recovered from the legacy index alone. In the value-based layout, the version marker would distinguish missing historical metadata from intentionally absent kinds in newly written records. In a separate-index layout, readiness would establish new-index coverage before the kind-aware cache was used for queries.

The implementation PR must update [ADR-005 (Badger storage's record layouts)](../adr/005-badger-storage-record-layouts.md), including its `0x82` section. That update must describe the implemented key and value format, the role of the legacy index during transition, and the cache changes. If field boundaries are addressed, it must also correct the suggestion that an exact check of the concatenated prefix can distinguish different service/operation splits.

---

## 9. Implementation plan

1. **Settle the persistent representation.** Use the benchmark and the alternatives above to choose the record format. Specify field boundaries, kind encoding, and handling of repeated complete keys. Confirm the proposed prefix is available if a new one is needed.
2. **Update the existing writer, prefill, cache, and `GetOperations`.** Preserve name/kind pairs and expiry during ingestion and reconstruction, and apply the requested kind filter. Test missing and unknown kinds, repeated names, restarts, embedded control bytes, and decoding failures.
3. **Implement the gate and compatibility checks.** Add the per-database warm-up check and the read/write behavior for each gate state. Test upgrades with existing data, gate toggles, restored databases, and rollback with actual old and new binaries. A new index also needs dual-write and union-read tests covering ordering, limits, and deduplication.
4. **Complete integration and documentation.** Remove Badger's `getOperationsMissingSpanKind: true` opt-out in `capabilities.Badger()` in `internal/storage/integration/capabilities/capabilities.go` once the enabled configuration supports the behavior. Keep coverage for the disabled gate. Update ADR-005 and measure prefill using the final encoding.

---

## 10. Open questions

- **Should this change also replace the concatenated key?** Keeping `0x82` offers a smaller change to trace search. Separating fields would fix an existing ambiguity but require a transition to a new format.
- **How should a value-based layout retain multiple kinds under an identical complete key?** Different start times or trace IDs already separate ordinary records. If all key fields match, the implementation must preserve every contributing kind or use a record identity that distinguishes them.
- **What is the final kind encoding?** A versioned string value can preserve absent and unknown string kinds. A numeric kind in the key would need its own complete mapping, and non-string tags need an explicit conversion or rejection rule.
- **How will readiness be checked?** The implementation needs a database-specific way to establish coverage and invalidate it after legacy writes or restoration, before allowing the gate to be enabled.

---

## 11. References

1. [#1922: span kind support in operation queries](https://github.com/jaegertracing/jaeger/issues/1922).
2. [#6376: earlier implementation and dual-lookup discussion](https://github.com/jaegertracing/jaeger/pull/6376).
3. [Yuri's review of the key layout and value-based alternative](https://github.com/jaegertracing/jaeger/pull/8402#discussion_r3120670715).
4. [Badger preload benchmark](https://gist.github.com/Audio431/5dda84ae8189c60932d9e1156003204c).
5. [Benchmark results posted to the design discussion](https://github.com/jaegertracing/jaeger/pull/8402#discussion_r3169432632).
6. [Review covering document scope, encoding, and rollout](https://github.com/jaegertracing/jaeger/pull/8402#pullrequestreview-5644968984).
7. [ADR-005 (Badger storage's record layouts)](../adr/005-badger-storage-record-layouts.md).
8. [Badger writer](../../internal/storage/v1/badger/spanstore/writer.go), [reader](../../internal/storage/v1/badger/spanstore/reader.go), and [cache](../../internal/storage/v1/badger/spanstore/cache.go).
9. [Storage integration capabilities](../../internal/storage/integration/capabilities/capabilities.go).
