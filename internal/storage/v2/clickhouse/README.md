## Clickhouse

### Schema storage tuning

New tables created with `create_schema: true` use `ZSTD(1)` compression on all
columns and `Delta(8), ZSTD(1)` on the span's `start_time`. Repeated names, kinds,
status codes, instrumentation scope names and versions, and attribute keys use
`LowCardinality(String)`, including corresponding columns in the derived tables.
Attribute values retain their original types.

The `spans` table uses `index_granularity = 8192` and `ttl_only_drop_parts = 1`.
When `ttl` is configured, ClickHouse drops a part only after every row in it has
expired. Daily partitioning can therefore retain data for up to one extra day,
plus background TTL processing time. The unpartitioned `trace_id_timestamps`
table keeps row-level TTL so that newer trace bounds do not retain older ones.

Schema creation uses `CREATE TABLE IF NOT EXISTS`; it does not modify existing
tables or copy historical data. Operators using `create_schema: false` can use
the [DDL templates](sql/) when creating their tables.

### Differences from the implementation in [otel collector contrib](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/exporter/clickhouseexporter)

#### Trace Storage Format

The most significant difference lies in the handling of **Attributes**. In the OTel-contrib implementation, everything within the Attributes is converted to [strings](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/80b3df26b7028a4bbe1eb606a6142cd4df9c3c74/exporter/clickhouseexporter/internal/metrics_model.go#L171-L177):

```golang
func AttributesToMap(attributes pcommon.Map) column.IterableOrderedMap {
	return orderedmap.CollectN(func(yield func(string, string) bool) {
		for k, v := range attributes.All() {
			yield(k, v.AsString())
		}
	}, attributes.Len())
}
```

The primary reason for this is that it leads to the loss of the original data types and \~\~cannot be used directly as query parameters\~\~ (Clickhouse provides casting functions). For example, if an attribute has an `int64` value, we might want to perform the following operation:

```sql
SELECT * FROM test WHERE resource.attributes['container.restart.count'] > 10
```

To address the above issues, the following improvements have been implemented:

  * Instead of directly using a Map for storage, the key and value are split into two separate arrays.
  * More Columns are used to store values of different types:
      * For basic types like bool, double, int, and string, corresponding type array columns are used for storage: `Array(Int64)`, `Array(Bool)`, etc.
      * For complex types like slice and map, they are serialized into JSON format strings before storage: `Array(String)`.

The `Value` type here actually refers to the `pdata` data types from the `otel-collector` pipeline. In our architecture, the `value_warpper` is responsible for wrapping the Protobuf-generated Go structures (which are the concrete implementation of `pdata`) into the `Value` type. Although `pdata` itself is based on the OTLP specification, encapsulating it into `Value` via the `value_warpper` creates a higher-level abstraction, which presents some challenges for directly storing `Value` in ClickHouse. Specifically, when deserializing `Slice` and `Map` data contained within the `Value`, the fact that JSON cannot natively distinguish whether a `Number` is an integer (`int`) or a floating-point number (`double`) leads to a loss of type information. Furthermore, directly handling the potentially dynamically nested `pdata` structures within the `Value` can also be quite complex. Therefore, to ensure the accuracy and completeness of data types in ClickHouse, and to effectively handle these nested telemetry data, we need to convert the `pdata` data inside `Value` into the standard `OTLP/JSON` format for storage.

#### Data Read and Write Methods

The OTel-contrib implementation uses `database/sql` for writing data. Using the provided generic interface is unnecessary; using the client provided by Clickhouse is a better choice.
For write operations, `ch-go`'s `chpool` is used in batch mode. For read operations, `clickhouse-go` is used.
