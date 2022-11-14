package mappings

import (
	"fmt"
	"strconv"
)

type FieldType int

func ParseFieldType(v any) FieldType {
	if value, ok := v.(bool); ok {
		if value {
			return ObjectFieldType
		}
		return NestedFieldType
	} else if value, ok := v.(string); ok {
		switch value {
		case "object", "true":
			return ObjectFieldType
		default:
			return NestedFieldType
		}
	}
	return NestedFieldType
}

func (field FieldType) Format(f fmt.State, verb rune) {

	str := "object"
	if field == NestedFieldType {
		str = "nested"
	}

	switch verb {
	case 'v':
		_, _ = f.Write([]byte(str))
	default:
		_, _ = f.Write([]byte(strconv.Itoa(int(field))))

	}
}

const (
	NestedFieldType FieldType = iota
	ObjectFieldType
)
