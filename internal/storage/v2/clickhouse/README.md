# Differences from the implementation in [OTel collector contrib](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/exporter/clickhouseexporter)

## Trace Storage Format

The most significant difference lies in the handling of **Attributes**. 
In the OTel-contrib implementation, everything within the Attributes is converted to
[strings](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/80b3df26b7028a4bbe1eb606a6142cd4df9c3c74/exporter/clickhouseexporter/internal/metrics_model.go#L171-L177):

The primary reason for this is that it leads to the loss of the original data types and cannot be used directly as query parameters (Clickhouse provides casting functions). 
For example, if an attribute has an `int64` value, we might want to perform the following operation:



```
SELECT * FROM test WHERE resource.attributes\['container.restart.count'] > 10
```

To address the above issues, the following improvements have been implemented:

Instead of using the Clickhouse [Map](https://clickhouse.com/docs/sql-reference/data-types/map) data type to store the entire `Attributes`, 
it is split according to the [Value](https://github.com/open-telemetry/opentelemetry-collector/blob/main/pdata/pcommon/value.go#L17-L29) data type in the OTLP Map.

The OTLP Map is grouped into an array of keys and an array of values according to the [Value](https://github.com/open-telemetry/opentelemetry-collector/blob/7adca809a60fad65a71063f6651f231466a29844/pdata/pcommon/map.go#L15-L19) data type. 
For example, suppose an OTLP Map contains two key-value pairs with value types of string and integer: ("server.name" : "jaeger") and ("process.pid", 1234). Then the entire OTLP Map will be divided into 4 parts:
- All the keys of key-value pairs with the string value type are collected into the array `strKeys("server.name")`

- All the values of key-value pairs with the string value type are collected into the array `strValues("jaeger")`

- All the keys of key-value pairs with the integer value type are collected into the array `intKeys("process.pid")`

- All the values of key-value pairs with the integer value type are collected into the array `intValues(1234)`

⚠️**The current solution does not consider complex types** [Slice](https://github.com/open-telemetry/opentelemetry-collector/blob/7adca809a60fad65a71063f6651f231466a29844/pdata/pcommon/slice.go#L15-L22) and [Map](https://github.com/open-telemetry/opentelemetry-collector/blob/7adca809a60fad65a71063f6651f231466a29844/pdata/pcommon/map.go#L15-L19). 
The reason is the lack of correct serialization/deserialization methods. 
In addition, we have already submitted a corresponding [proposal](https://github.com/open-telemetry/opentelemetry-collector/issues/12826) to the OTel community. Once the community approves this improvement, we can support them quickly.⚠️

The expression form after grouping Attributes:
- For basic types like bool, double, int, string, bytes, we can directly store them as arrays: `Array(Bool)`, `Array(Int64)`, `Array(Float64)`, `Array(String)`, `Array(Array(Uint8))`
- For complex types like slice and map, they are serialized into JSON format strings before storage: `Array(String)`

### How to handle complex types (Map, Slice)

The `Value` type here actually refers to the `pdata` data types in the `otel-collector` pipeline. In our architecture, 
the `value_warpper` is responsible for wrapping the Protobuf-generated Go structures (which are the concrete implementation of `pdata`) into the `Value` type.
Although `pdata` itself is based on the OTLP specification, encapsulating it into `Value` via the `value_warpper` creates a higher-level abstraction, 
which presents some challenges for directly storing `Value` in ClickHouse. Specifically, when deserializing `Slice` and `Map` data contained within the `Value`, 
the fact that JSON cannot natively distinguish whether a `Number` is an integer (`int`) or a floating-point number (`double`) leads to a loss of type information. 
Furthermore, directly handling the potentially dynamically nested `pdata` structures within the `Value` can also be quite complex. 
Therefore, to ensure the accuracy and completeness of data types in ClickHouse, and to effectively handle these nested telemetry data, 
we need to convert the `pdata` data inside `Value` into the standard `OTLP/JSON` format for storage.

### Data Read and Write Methods

The OTel-contrib implementation uses `database/sql` for writing data. Using the provided generic interface is unnecessary; using the client provided by ClickHouse is a better choice.

For write operations and read operations, `clickhouse-go` is used in batch mode for writing traces and retrieving traces.

The main reason for not using `ch-go` to write traces is that `ch-go` doesn't support multi-server writing. It may cause performance bottlenecks.

## Mapping model to DB storage
The table structure is defined: [internal/storage/v2/clickhouse/schema/schema.tmpl](./schema/schema.tmpl).

### Attributes
For `Attributes`, the key-value pairs are split into separate Key and Value columns based on their data type.

For example, `SpanAttrBoolKey` stores the keys of boolean type attributes, and `SpanAttrBoolValue` stores their corresponding values.
As for `Events` and `Links`, logically, they should be one-to-many `Nested` structures.
Similarly, we use `Nested` to store their Attributes.

### TraceID,SpanID
`TraceID` is actually a fixed-length `[16]byte`, and `SpanID` is a fixed-length `[8]byte`. They are represented as `byte` slices in Go but stored using the column `Array(UInt8)` in the database. Note that the way they represent bytes is different: Clickhouse uses [int8](https://clickhouse.com/docs/sql-reference/data-types/int-uint#integer-aliases), while Go uses uint8.