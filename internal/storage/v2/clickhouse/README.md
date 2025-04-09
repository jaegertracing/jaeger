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

We cannot directly use `encode/json` to serialize the values, as this would also lead to the loss of type information. For instance, double and int values would be converted into JSON numbers, and during deserialization, we wouldn't know whether it was originally an int or a double. Similarly, for deeply nested slices and maps, we cannot predict their structure (slice, slice(slice), slice(slice(slice)), etc.).
Fortunately, the [`Marshal`](https://www.google.com/search?q=%5Bhttps://github.com/open-telemetry/opentelemetry-collector/blob/pdata/v1.29.0/pdata/internal/json/json.go%23L20) and [`ReadValue`](https://www.google.com/search?q=%5Bhttps://pkg.go.dev/go.opentelemetry.io/collector/pdata/internal/json%23ReadValue%5D) provided by Otel perfectly solves this problem.

#### Data Read and Write Methods

The OTel-contrib implementation uses `database/sql` for writing data. Using the provided generic interface is unnecessary; using the client provided by Clickhouse is a better choice.
For write operations, `ch-go`'s `chpool` is used in batch mode. For read operations, `clickhouse-go` is used.