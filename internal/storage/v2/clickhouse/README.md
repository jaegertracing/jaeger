## Clickhouse

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

### Hoisted attributes

A deployment can copy attributes that it searches often into dedicated columns of the `spans` table. The backend calls such an attribute "hoisted". Each entry of `hoisted_attributes` names one attribute by its key, level and stored type, and the column that receives the copy:

```yaml
hoisted_attributes:
  - key: gen_ai.request.model
    level: span
    type: string
    column: gen_ai_request_model
  - key: gen_ai.usage.input_tokens
    level: span
    type: int
    column: input_tokens
    codec: ZSTD(3)
```

| Field | Values | Meaning |
| --- | --- | --- |
| `key` | any non-empty string | The attribute key. It is an opaque string, never a column name or an expression. |
| `level` | `span`, `resource` | Where the attribute is stored. |
| `type` | `string`, `int`, `double`, `bool` | The stored type of the attribute. It selects the typed group to copy from and converts nothing: a value stored under the same key with another type is not copied. The same key may be listed once per type. |
| `column` | `[a-z_][a-z0-9_]*` | The name of the dedicated column. It must not be a column the table already has. |
| `codec` | `ZSTD(1)` to `ZSTD(22)` | Optional compression codec of the column. The default is `ZSTD(1)`. |

The column is `MATERIALIZED`: ClickHouse computes it from the typed attribute group when a span is inserted. Its type is `Nullable`, so a span without the attribute holds `NULL` and not zero, `false` or an empty string. The typed groups stay the source of the attribute; a trace is still read from them, and `attribute_metadata` still records the attribute.

An equality search on a hoisted attribute compares the column instead of scanning the key and value arrays, in both the tag search and the structured filter. The same key at another level or with another type, and every attribute that is not hoisted, is searched as before. The copy costs the storage of a second copy of the value and the CPU to compute it on insert.

Two properties follow from the copy holding one value per span:

- If a span carries the same key twice in one typed group, which OTLP forbids, the column holds the first value, and a search for the second finds the span only when the attribute is not hoisted.
- The backend creates no index on the column. An operator who wants granule pruning adds a skip index (`bloom_filter`, `minmax`, or another type that suits the values) and materializes it for existing parts.

#### Adding a hoisted attribute to an existing table

With `create_schema: true`, a table that the backend creates has the columns. `CREATE TABLE IF NOT EXISTS` never changes an existing table, so on an existing table, and always with `create_schema: false`, the operator adds each column before the entry goes into the configuration:

```sql
ALTER TABLE spans ADD COLUMN IF NOT EXISTS input_tokens Nullable(Int64)
    MATERIALIZED if(has(int_attributes.key, 'gen_ai.usage.input_tokens'),
                    int_attributes.value[indexOf(int_attributes.key, 'gen_ai.usage.input_tokens')],
                    NULL)
    CODEC(ZSTD(3));
```

At startup the backend reads every configured column from `system.columns` and refuses to start when a column is missing or its type, expression or codec differs from what the configuration renders. The error for a missing column contains the exact `ALTER TABLE` statement to run.

Parts written before the `ALTER` do not store the column, and ClickHouse computes it from the attribute groups each time such a part is read, which returns correct results at the old cost. To store the column in existing parts, run the mutation and wait for it to finish:

```sql
ALTER TABLE spans MATERIALIZE COLUMN input_tokens;
SELECT mutation_id, parts_to_do, is_done FROM system.mutations WHERE database = currentDatabase() AND table = 'spans' AND NOT is_done;
```

The backend never starts this mutation itself.

#### Removing or changing a hoisted attribute

Removing an entry from the configuration makes searches use the typed groups again. The column stays in the table until the operator drops it. Changing the key, level, type or codec of an entry is a schema change: drop the column and add it again with the new definition, or the startup check refuses the mismatch.
