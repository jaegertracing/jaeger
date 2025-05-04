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

The primary reason for this is that it leads to the loss of the original data types and cannot be used directly as query parameters (Clickhouse provides casting functions). For example, if an attribute has an `int64` value, we might want to perform the following operation:

```sql
SELECT * FROM test WHERE resource.attributes['container.restart.count'] > 10
```

To address the above issues, the following improvements have been implemented:

  * Instead of directly using a Map for storage, the key and value are split into two separate arrays.
  * More Columns are used to store values of different types:
      * For basic types like bool, double, int, and string, corresponding type array columns are used for storage: `Array(Int64)`, `Array(Bool)`, etc.
      * For complex types like slice and map, they are serialized into JSON format strings before storage: `Array(String)`.

The `Value` type here actually refers to the `pdata` data types from the `otel-collector` pipeline. In our architecture, the `value_warpper` is responsible for wrapping the Protobuf-generated Go structures (which are the concrete implementation of `pdata`) into the `Value` type. Although `pdata` itself is based on the OTLP specification, encapsulating it into `Value` via the `value_warpper` creates a higher-level abstraction, which presents some challenges for directly storing `Value` in ClickHouse. Specifically, when deserializing `Slice` and `Map` data contained within the `Value`, the fact that JSON cannot natively distinguish whether a `Number` is an integer (`int`) or a floating-point number (`double`) leads to a loss of type information. Furthermore, directly handling the potentially dynamically nested `pdata` structures within the `Value` can also be quite complex. Therefore, to ensure the accuracy and completeness of data types in ClickHouse, and to effectively handle these nested telemetry data, we need to convert the `pdata` data inside `Value` into the standard `OTLP/JSON` format for storage.
#### Mapping model to DB storage
The table structure is defined as follows:
``` sql
    Timestamp DateTime64(9) CODEC(Delta, ZSTD(1)),
    TraceID Array(byte) CODEC(ZSTD(1)),
    SpanID Array(byte) CODEC(ZSTD(1)),
    ParentSpanID Array(byte) CODEC(ZSTD(1)),
    TraceState String CODEC(ZSTD(1)),
    SpanName String CODEC(ZSTD(1)),
    SpanKind String CODEC(ZSTD(1)),
    Duration DateTime64(9) CODEC(Delta, ZSTD(1)),
    StatusCode String CODEC(ZSTD(1)),
    StatusMessage String CODEC(ZSTD(1)),
    
    SpanAttributesBoolKey Array(String),
    SpanAttributesBoolValue Array(Bool),
    SpanAttributesDoubleKey Array(String),
    SpanAttributesDoubleValue Array(Float64),
    SpanAttributesIntKey Array(String),
    SpanAttributesIntValue Array(Int64),
    SpanAttributesStrKey Array(String),
    SpanAttributesStrValue Array(String),
    SpanAttributesBytesKey Array(String),
    SpanAttributesBytesValue Array(Array(byte)),
    
    ScopeName String CODEC(ZSTD(1)),
    ScopeVersion String CODEC(ZSTD(1)),
    ScopeAttributesBoolKey Array(String),
    ScopeAttributesBoolValue Array(Bool),
    ScopeAttributesDoubleKey Array(String),
    ScopeAttributesDoubleValue Array(Float64),
    ScopeAttributesIntKey Array(String),
    ScopeAttributesIntValue Array(Int64),
    ScopeAttributesStrKey Array(String),
    ScopeAttributesStrValue Array(String),
    ScopeAttributesBytesKey Array(String),
    ScopeAttributesBytesValue Array(Array(byte)),
    
    ResourceAttributesBoolKey Array(String),
    ResourceAttributesBoolValue Array(Bool),
    ResourceAttributesDoubleKey Array(String),
    ResourceAttributesDoubleValue Array(Float64),
    ResourceAttributesIntKey Array(String),
    ResourceAttributesIntValue Array(Int64),
    ResourceAttributesStrKey Array(String),
    ResourceAttributesStrValue Array(String),
    ResourceAttributesBytesKey Array(String),
    ResourceAttributesBytesValue Array(Array(byte)),
    
    EventsName Array(String),
    EventsTimestamp Array(DateTime64(9)) CODEC(Delta, ZSTD(1)),
    EventsAttributesBoolKeys Array(Array(String)),
    EventsAttributesBoolValues Array(Array(Bool)),
    EventsAttributesDoubleKeys Array(Array(String)),
    EventsAttributesDoubleValues Array(Array(Float64)),
    EventsAttributesIntKeys Array(Array(String)),
    EventsAttributesIntValues Array(Array(Int64)),
    EventsAttributesStrKeys Array(Array(String)),
    EventsAttributesStrValues Array(Array(String)),
    EventsAttributesBytesKeys Array(Array(String)),
    EventsAttributesBytesValues Array(Array(Array(byte))),
    
    LinksTraceId Array(Array(byte)),
    LinksSpanId Array(Array(byte)),
    LinksTraceStatus Array(String),
    LinksAttributesBoolKeys Array(Array(String)),
    LinksAttributesBoolValues Array(Array(Bool)),
    LinksAttributesDoubleKeys Array(Array(String)),
    LinksAttributesDoubleValues Array(Array(Float64)),
    LinksAttributesIntKeys Array(Array(String)),
    LinksAttributesIntValues Array(Array(Int64)),
    LinksAttributesStrKeys Array(Array(String)),
    LinksAttributesStrValues Array(Array(String)),
    LinksAttributesBytesKeys Array(Array(String)),
    LinksAttributesBytesValues Array(Array(Array(byte))),
```
`TraceID` is actually a fixed-length `[16]byte`, and `SpanID` is a fixed-length `[8]byte`. 
While it's possible to store them using `Array(byte)`, using the `FixedString(16)` and `FixedString(8)` types can express the data length more precisely.

Furthermore, for all xxxAttributes, the key-value pairs are split into separate Key and Value columns based on their data type.
For example, `SpanAttributesBoolKey` stores the keys of boolean type attributes, and `SpanAttributesBoolValue` stores their corresponding values.

As for `Events` and `Links`, they should logically be one-to-many `Nested` structures. 
However, because their associated Attributes have been split into multiple independent arrays (*_Key and *_Value columns), 
and the number of values in these arrays may not correspond one-to-one with the number of Events or Links, 
it's not possible to directly use the `Nested` type to represent this relationship.

#### Data Read and Write Methods
The OTel-contrib implementation uses `database/sql` for writing data. Using the provided generic interface is unnecessary; 
using the client provided by ClickHouse is a better choice.
For write operations and read operations, `clickhouse-go` is used in batch mode for writing traces and retrieving traces. 
The main reason for not using `ch-go` to write traces is that `ch-go` doesn't support multi-server writing. It may cause performance bottlenecks.