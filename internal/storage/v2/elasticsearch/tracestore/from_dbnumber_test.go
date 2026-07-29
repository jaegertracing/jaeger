package tracestore

import (
    "testing"
    "github.com/jaegertracing/jaeger/model"
    "go.opentelemetry.io/collector/pdata/pcommon"
)

func TestFromDBNumber_Int64(t *testing.T) {
    kv := model.KeyValue{Key: "int64_key", Type: model.Int64Type, AsInt64: 42}
    attrs := pcommon.NewMap()
    fromDBNumber(kv, attrs)
    if v, ok := attrs.Get("int64_key"); !ok || v.Type() != pcommon.ValueTypeInt || v.Int() != 42 {
        t.Fatalf("expected int64 attribute with value 42, got %v (exists=%v)", v, ok)
    }
}

func TestFromDBNumber_Float64(t *testing.T) {
    kv := model.KeyValue{Key: "float64_key", Type: model.Float64Type, AsFloat64: 3.14}
    attrs := pcommon.NewMap()
    fromDBNumber(kv, attrs)
    if v, ok := attrs.Get("float64_key"); !ok || v.Type() != pcommon.ValueTypeDouble || v.Double() != 3.14 {
        t.Fatalf("expected float64 attribute with value 3.14, got %v (exists=%v)", v, ok)
    }
}

func TestFromDBNumber_Bool(t *testing.T) {
    kv := model.KeyValue{Key: "bool_key", Type: model.BoolType, AsBool: true}
    attrs := pcommon.NewMap()
    fromDBNumber(kv, attrs)
    if v, ok := attrs.Get("bool_key"); !ok || v.Type() != pcommon.ValueTypeBool || v.Bool() != true {
        t.Fatalf("expected bool attribute true, got %v (exists=%v)", v, ok)
    }
}
