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

// IsLess compares two KeyValue objects. The order is based first on the keys, then on type, and finally on the value.
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
	case BoolType, Int64Type:
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
