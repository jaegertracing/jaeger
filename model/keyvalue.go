package model

import "math"

// ValueType describes the type of value contained in a KeyValue struct
type ValueType int

const (
	// StringType indicates the value is a unicode string
	StringType ValueType = iota
	// BoolType indicates the value is a Boolean encoded as int64 number 0 or 1
	BoolType
	// Int64Type indicates the value is an int64 number
	Int64Type
	// Float64Type indicates the value is a float64 number stored as int64
	Float64Type
	// BinaryType indicates the value is binary blob stored as a byte array
	BinaryType
)

// KeyValue describes a tag or a log field that consists of a key and a value
type KeyValue struct {
	Key   string    `json:"key"`
	VType ValueType `json:"vType"`
	VStr  string    `json:"vStr,omitempty"`
	VNum  int64     `json:"vNum,omitempty"`
	VBlob []byte    `json:"vBlob,omitempty"`
}

func String(key string, value string) KeyValue {
	return KeyValue{Key: key, VType: StringType, VStr: value}
}

func Bool(key string, value bool) KeyValue {
	var val int64
	if value {
		val = 1
	}
	return KeyValue{Key: key, VType: BoolType, VNum: val}
}

func Int64(key string, value int64) KeyValue {
	return KeyValue{Key: key, VType: Int64Type, VNum: value}
}

func Float64(key string, value float64) KeyValue {
	return KeyValue{Key: key, VType: Float64Type, VNum: int64(math.Float64bits(value))}
}

func Binary(key string, value []byte) KeyValue {
	return KeyValue{Key: key, VType: BinaryType, VBlob: value}
}

func (kv *KeyValue) Bool() bool {
	if kv.VType == BoolType {
		return kv.VNum == 1
	}
	return false
}

func (kv *KeyValue) Int64() int64 {
	if kv.VType == Int64Type {
		return kv.VNum
	}
	return 0
}

func (kv *KeyValue) Float64() float64 {
	if kv.VType == Float64Type {
		return math.Float64frombits(uint64(kv.VNum))
	}
	return 0
}

func (kv *KeyValue) Binary() []byte {
	if kv.VType == BinaryType {
		return kv.VBlob
	}
	return nil
}

// IsLess compares two KeyValue objects. The order is based first on the keys, then on type, and finally no the value.
func IsLess(one *KeyValue, two *KeyValue) bool {
	if one.Key != two.Key {
		return one.Key < two.Key
	}
	if one.VType != two.VType {
		return one.VType < two.VType
	}
	switch one.VType {
	case StringType:
		return one.VStr < two.VStr
	case BoolType:
		return one.VNum < two.VNum
	case Int64Type:
		return one.VNum < two.VNum
	case Float64Type:
		return one.Float64() < two.Float64()
	case BinaryType:
		l1, l2 := len(one.VBlob), len(two.VBlob)
		minLen := l1
		if l2 < minLen {
			minLen = l2
		}
		for i := 0; i < minLen; i++ {
			if d := int(one.VBlob[i]) - int(two.VBlob[i]); d != 0 {
				return d < 0
			}
		}
		if l1 == l2 {
			return false
		}
		return l1 < l2
	default:
		return false
	}
}
