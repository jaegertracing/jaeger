package jpcommon

import (
	"github.com/jaegertracing/jaeger/proto-gen/storage/v2"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

func PcommonMapToPlainMap(attributes pcommon.Map) map[string]string {
	mapAttributes := make(map[string]string)
	attributes.Range(func(k string, v pcommon.Value) bool {
		mapAttributes[k] = v.AsString()
		return true
	})
	return mapAttributes
}

func PlainMapToPcommonMap(attributesMap map[string]string) pcommon.Map {
	attributes := pcommon.NewMap()
	for k, v := range attributesMap {
		attributes.PutStr(k, v)
	}
	return attributes
}

// ConvertMapToKeyValues converts a pcommon.Map to []*storage.KeyValue.
func ConvertMapToKeyValueList(m pcommon.Map) []*storage.KeyValue {
	keyValues := make([]*storage.KeyValue, 0, m.Len())
	m.Range(func(k string, v pcommon.Value) bool {
		keyValues = append(keyValues, &storage.KeyValue{
			Key:   k,
			Value: convertValueToAnyValue(v),
		})
		return true
	})
	return keyValues
}

func convertValueToAnyValue(v pcommon.Value) *storage.AnyValue {
	switch v.Type() {
	case pcommon.ValueTypeStr:
		return &storage.AnyValue{
			Value: &storage.AnyValue_StringValue{
				StringValue: v.Str(),
			},
		}
	case pcommon.ValueTypeBool:
		return &storage.AnyValue{
			Value: &storage.AnyValue_BoolValue{
				BoolValue: v.Bool(),
			},
		}
	case pcommon.ValueTypeInt:
		return &storage.AnyValue{
			Value: &storage.AnyValue_IntValue{
				IntValue: v.Int(),
			},
		}
	case pcommon.ValueTypeDouble:
		return &storage.AnyValue{
			Value: &storage.AnyValue_DoubleValue{
				DoubleValue: v.Double(),
			},
		}
	case pcommon.ValueTypeBytes:
		return &storage.AnyValue{
			Value: &storage.AnyValue_BytesValue{
				BytesValue: v.Bytes().AsRaw(),
			},
		}
	case pcommon.ValueTypeSlice:
		arr := v.Slice()
		arrayValues := make([]*storage.AnyValue, 0, arr.Len())
		for i := 0; i < arr.Len(); i++ {
			arrayValues = append(arrayValues, convertValueToAnyValue(arr.At(i)))
		}
		return &storage.AnyValue{
			Value: &storage.AnyValue_ArrayValue{
				ArrayValue: &storage.ArrayValue{
					Values: arrayValues,
				},
			},
		}
	case pcommon.ValueTypeMap:
		kvList := &storage.KeyValueList{}
		v.Map().Range(func(k string, val pcommon.Value) bool {
			kvList.Values = append(kvList.Values, &storage.KeyValue{
				Key:   k,
				Value: convertValueToAnyValue(val),
			})
			return true
		})
		return &storage.AnyValue{Value: &storage.AnyValue_KvlistValue{KvlistValue: kvList}}
	default:
		return nil
	}
}
