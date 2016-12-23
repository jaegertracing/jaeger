// Copyright (c) 2016 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package model

import (
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strconv"
)

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

	stringTypeStr  = "string"
	boolTypeStr    = "bool"
	int64TypeStr   = "int64"
	float64TypeStr = "float64"
	binaryTypeStr  = "binary"
)

// KeyValue describes a tag or a log field that consists of a key and a typed value.
// Before accessing a value, the caller must check the type. Boolean and numeric
// values should be accessed via accessor methods Bool(), Int64(), and Float64().
//
// This struct is designed to minimize heap allocations.
type KeyValue struct {
	Key   string    `json:"key"`
	VType ValueType `json:"vType"`
	VStr  string    `json:"vStr,omitempty"`
	VNum  int64     `json:"vNum,omitempty"`
	VBlob []byte    `json:"vBlob,omitempty"`
}

// KeyValues is a type alias that exposes convenience functions like Sort, FindByKey.
type KeyValues []KeyValue

// String creates a String-typed KeyValue
func String(key string, value string) KeyValue {
	return KeyValue{Key: key, VType: StringType, VStr: value}
}

// Bool creates a Bool-typed KeyValue
func Bool(key string, value bool) KeyValue {
	var val int64
	if value {
		val = 1
	}
	return KeyValue{Key: key, VType: BoolType, VNum: val}
}

// Int64 creates a Int64-typed KeyValue
func Int64(key string, value int64) KeyValue {
	return KeyValue{Key: key, VType: Int64Type, VNum: value}
}

// Float64 creates a Float64-typed KeyValue
func Float64(key string, value float64) KeyValue {
	return KeyValue{Key: key, VType: Float64Type, VNum: int64(math.Float64bits(value))}
}

// Binary creates a Binary-typed KeyValue
func Binary(key string, value []byte) KeyValue {
	return KeyValue{Key: key, VType: BinaryType, VBlob: value}
}

// Bool returns the Boolean value stored in this KeyValue or false if it stores a different type.
// The caller must check VType before using this method.
func (kv *KeyValue) Bool() bool {
	if kv.VType == BoolType {
		return kv.VNum == 1
	}
	return false
}

// Int64 returns the Int64 value stored in this KeyValue or 0 if it stores a different type.
// The caller must check VType before using this method.
func (kv *KeyValue) Int64() int64 {
	if kv.VType == Int64Type {
		return kv.VNum
	}
	return 0
}

// Float64 returns the Float64 value stored in this KeyValue or 0 if it stores a different type.
// The caller must check VType before using this method.
func (kv *KeyValue) Float64() float64 {
	if kv.VType == Float64Type {
		return math.Float64frombits(uint64(kv.VNum))
	}
	return 0
}

// Binary returns the blob ([]byte) value stored in this KeyValue or nil if it stores a different type.
// The caller must check VType before using this method.
func (kv *KeyValue) Binary() []byte {
	if kv.VType == BinaryType {
		return kv.VBlob
	}
	return nil
}

// AsString returns a potentially lossy string representation of the value.
func (kv *KeyValue) AsString() string {
	switch kv.VType {
	case StringType:
		return kv.VStr
	case BoolType:
		if kv.Bool() {
			return "true"
		}
		return "false"
	case Int64Type:
		return strconv.FormatInt(kv.Int64(), 10)
	case Float64Type:
		return strconv.FormatFloat(kv.Float64(), 'g', 10, 64)
	case BinaryType:
		if len(kv.VBlob) > 16 {
			return hex.EncodeToString(kv.VBlob[0:16]) + "..."
		}
		return hex.EncodeToString(kv.VBlob)
	default:
		return fmt.Sprintf("unknown type %d", kv.VType)
	}
}

// Equal compares KeyValue object with another KeyValue.
func (kv *KeyValue) Equal(other *KeyValue) bool {
	if kv.Key != other.Key {
		return false
	}
	if kv.VType != other.VType {
		return false
	}
	switch kv.VType {
	case StringType:
		return kv.VStr == other.VStr
	case BoolType, Int64Type:
		return kv.VNum == other.VNum
	case Float64Type:
		return kv.Float64() == other.Float64()
	case BinaryType:
		l1, l2 := len(kv.VBlob), len(other.VBlob)
		if l1 != l2 {
			return false
		}
		for i := 0; i < l1; i++ {
			if kv.VBlob[i] != other.VBlob[i] {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// IsLess compares KeyValue object with another KeyValue.
// The order is based first on the keys, then on type, and finally on the value.
func (kv *KeyValue) IsLess(two *KeyValue) bool {
	if kv.Key != two.Key {
		return kv.Key < two.Key
	}
	if kv.VType != two.VType {
		return kv.VType < two.VType
	}
	switch kv.VType {
	case StringType:
		return kv.VStr < two.VStr
	case BoolType, Int64Type:
		return kv.VNum < two.VNum
	case Float64Type:
		return kv.Float64() < two.Float64()
	case BinaryType:
		l1, l2 := len(kv.VBlob), len(two.VBlob)
		minLen := l1
		if l2 < minLen {
			minLen = l2
		}
		for i := 0; i < minLen; i++ {
			if d := int(kv.VBlob[i]) - int(two.VBlob[i]); d != 0 {
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

func (kvs KeyValues) Len() int      { return len(kvs) }
func (kvs KeyValues) Swap(i, j int) { kvs[i], kvs[j] = kvs[j], kvs[i] }
func (kvs KeyValues) Less(i, j int) bool {
	return kvs[i].IsLess(&kvs[j])
}

// Sort does in-place sorting of KeyValues, then by value type, then by value.
func (kvs KeyValues) Sort() {
	sort.Sort(kvs)
}

// FindByKey scans the list of key-values searching for the first one with the given key.
// Returns found tag and a boolean flag indicating if the search was successful.
func (kvs KeyValues) FindByKey(key string) (KeyValue, bool) {
	for _, kv := range kvs {
		if kv.Key == key {
			return kv, true
		}
	}
	return KeyValue{}, false
}

// Equal compares KyValues with another list. Both lists must be already sorted.
func (kvs KeyValues) Equal(other KeyValues) bool {
	l1, l2 := len(kvs), len(other)
	if l1 != l2 {
		return false
	}
	for i := 0; i < l1; i++ {
		if !kvs[i].Equal(&other[i]) {
			return false
		}
	}
	return true
}

func (p ValueType) String() string {
	switch p {
	case StringType:
		return stringTypeStr
	case BoolType:
		return boolTypeStr
	case Int64Type:
		return int64TypeStr
	case Float64Type:
		return float64TypeStr
	case BinaryType:
		return binaryTypeStr
	}
	return "<invalid>"
}

// ValueTypeFromString converts a string into ValueType enum.
func ValueTypeFromString(s string) (ValueType, error) {
	switch s {
	case stringTypeStr:
		return StringType, nil
	case boolTypeStr:
		return BoolType, nil
	case int64TypeStr:
		return Int64Type, nil
	case float64TypeStr:
		return Float64Type, nil
	case binaryTypeStr:
		return BinaryType, nil
	}
	return ValueType(0), fmt.Errorf("not a valid ValueType string %s", s)
}

// MarshalText allows ValueType to serialize itself in JSON as a string.
func (p ValueType) MarshalText() ([]byte, error) {
	return []byte(p.String()), nil
}

// UnmarshalText allows ValueType to deserialize itself from a JSON string.
func (p *ValueType) UnmarshalText(text []byte) error {
	q, err := ValueTypeFromString(string(text))
	if err != nil {
		return err
	}
	*p = q
	return nil
}
