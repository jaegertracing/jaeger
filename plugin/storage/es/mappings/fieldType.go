package mappings

import (
	"fmt"
	"strconv"
)

type FieldType int

func ParseFieldType(v string) FieldType {
	switch v {
	case "object":
		return ObjectFieldType
	default:
		return NestedFieldType
	}
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
