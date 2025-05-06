## Clickhouse

### Differences from the implementation in [otel collector contrib](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/exporter/clickhouseexporter)

#### Trace Storage Format

The most significant difference lies in the handling of **Attributes**. In the OTel-contrib implementation, everything within the Attributes is converted to [strings](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/80b3df26b7028a4bbe1eb606a6142cd4df9c3c74/exporter/clickhouseexporter/internal/metrics_model.go#L171-L177):

The primary reason for this is that it leads to the loss of the original data types and cannot be used directly as query parameters (Clickhouse provides casting functions). For example, if an attribute has an `int64` value, we might want to perform the following operation:

```sql
SELECT * FROM test WHERE resource.attributes['container.restart.count'] > 10
```

To address the above issues, the following improvements have been implemented:

  * For `Resource` `Scope` `Span` Instead of directly using a Map for storage, the key and value are split into two separate arrays.
  * For `Events` `Links` using `Nested` (key,value).
  * More Columns are used to store values of different types:
      * For basic types like bool, double, int, and string, corresponding type array columns are used for storage: `Array(Int64)`, `Array(Bool)`, etc.
      * For complex types like slice and map, they are serialized into JSON format strings before storage: `Array(String)`.

The `Value` type here actually refers to the `pdata` data types from the `otel-collector` pipeline. In our architecture, the `value_warpper` is responsible for wrapping the Protobuf-generated Go structures (which are the concrete implementation of `pdata`) into the `Value` type. Although `pdata` itself is based on the OTLP specification, encapsulating it into `Value` via the `value_warpper` creates a higher-level abstraction, which presents some challenges for directly storing `Value` in ClickHouse. Specifically, when deserializing `Slice` and `Map` data contained within the `Value`, the fact that JSON cannot natively distinguish whether a `Number` is an integer (`int`) or a floating-point number (`double`) leads to a loss of type information. Furthermore, directly handling the potentially dynamically nested `pdata` structures within the `Value` can also be quite complex. Therefore, to ensure the accuracy and completeness of data types in ClickHouse, and to effectively handle these nested telemetry data, we need to convert the `pdata` data inside `Value` into the standard `OTLP/JSON` format for storage.
#### Mapping model to DB storage
The table structure is defined: [internal/storage/v2/clickhouse/schema/schema.tmpl](./schema/schema.tmpl)
.
We categorize Trace into ordinary types (Resource, Scope, Span) and complex types (Events, Links) based on quantity. 
Fields within ordinary types are recorded using conventional data types, while complex types are recorded using Nested data types.

##### Resource,Scope,Span
`TraceID` is actually a fixed-length `[16]byte`, and `SpanID` is a fixed-length `[8]byte`. 
While it's possible to store them using `Array(UInt8)`, 
for `Attributes`, the key-value pairs are split into separate Key and Value columns based on their data type.
For example, `SpanAttrBoolKey` stores the keys of boolean type attributes, and `SpanAttrBoolValue` stores their corresponding values.
更多内容看:[internal/storage/v2/clickhouse/tracestore/dbmodel/dbmodel.go](./tracestore/dbmodel/dbmodel.go)

#### Events,Links
As for `Events` and `Links`, they should logically be one-to-many `Nested` structures. 
Similarly, we use `Nested` to store their Attributes.
For more details, see: [internal/storage/v2/clickhouse/tracestore/dbmodel/row.go](./tracestore/dbmodel/row.go)
#### Data Read and Write Methods
The OTel-contrib implementation uses `database/sql` for writing data. Using the provided generic interface is unnecessary; 
using the client provided by ClickHouse is a better choice.
For write operations and read operations, `clickhouse-go` is used in batch mode for writing traces and retrieving traces. 
The main reason for not using `ch-go` to write traces is that `ch-go` doesn't support multi-server writing. It may cause performance bottlenecks.