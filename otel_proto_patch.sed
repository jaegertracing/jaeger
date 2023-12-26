# Substitute Go package name.
s|go.opentelemetry.io/proto/otlp|github.com/jaegertracing/jaeger/proto-gen/otel|g

# Substitute Proto package name.
s| opentelemetry.proto| jaeger|g

# Remove opentelemetry/proto prefix from imports.
s|import "opentelemetry/proto/|import "|g

# Below instructions are copied (and modified) from OTel collector
# https://github.com/open-telemetry/opentelemetry-collector/blob/main/proto_patch.sed
s|^package \(.*\)\;|package \1;\
\
import "gogoproto/gogo.proto";\

s+bytes trace_id = \(.*\);+bytes trace_id = \1\
  [\
  // Use custom TraceId data type for this field.\
  (gogoproto.nullable) = false,\
  (gogoproto.customtype) = "go.opentelemetry.io/collector/pdata/internal/data.TraceID"\
  ];+g

s+bytes \(.*span_id\) = \(.*\);+bytes \1 = \2\
  [\
  // Use custom SpanId data type for this field.\
  (gogoproto.nullable) = false,\
  (gogoproto.customtype) = "go.opentelemetry.io/collector/pdata/internal/data.SpanID"\
  ];+g

s+repeated opentelemetry.proto.common.v1.KeyValue \(.*\);+repeated opentelemetry.proto.common.v1.KeyValue \1\
  [ (gogoproto.nullable) = false ];+g

s+repeated KeyValue \(.*\);+repeated KeyValue \1\
  [ (gogoproto.nullable) = false ];+g

s+AnyValue \(.*\);+AnyValue \1\
  [ (gogoproto.nullable) = false ];+g

s+opentelemetry.proto.resource.v1.Resource resource = \(.*\);+opentelemetry.proto.resource.v1.Resource resource = \1\
  [ (gogoproto.nullable) = false ];+g

s+opentelemetry.proto.common.v1.InstrumentationScope scope = \(.*\);+opentelemetry.proto.common.v1.InstrumentationScope scope = \1\
  [ (gogoproto.nullable) = false ];+g

s+Status \(.*\);+Status \1\
  [ (gogoproto.nullable) = false ];+g

s+repeated Exemplar exemplars = \(.*\);+repeated Exemplar exemplars = \1\
  [ (gogoproto.nullable) = false ];+g

s+Buckets \(.*\)tive = \(.*\);+Buckets \1tive = \2\
  [ (gogoproto.nullable) = false ];+g

# optional fixed64 foo = 1 -> oneof foo_ { fixed64 foo = 1;}
s+optional \(.*\) \(.*\) = \(.*\);+ oneof \2_ { \1 \2 = \3;}+g

s+\(.*\)PartialSuccess partial_success = \(.*\);+\1PartialSuccess partial_success = \2\
  [ (gogoproto.nullable) = false ];+g
