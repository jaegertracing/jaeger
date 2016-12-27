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

package model_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/uber/jaeger/model"
)

func TestKeyValuesSort(t *testing.T) {
	input := model.KeyValues{
		model.String("x", "z"),
		model.String("x", "y"),
		model.Int64("a", 2),
		model.Int64("a", 1),
		model.Float64("x", 2),
		model.Float64("x", 1),
		model.Bool("x", true),
		model.Bool("x", false),
	}
	expected := model.KeyValues{
		model.Int64("a", 1),
		model.Int64("a", 2),
		model.String("x", "y"),
		model.String("x", "z"),
		model.Bool("x", false),
		model.Bool("x", true),
		model.Float64("x", 1),
		model.Float64("x", 2),
	}
	input.Sort()
	assert.Equal(t, expected, input)
}

func TestKeyValuesFindByKey(t *testing.T) {
	input := model.KeyValues{
		model.String("x", "z"),
		model.String("x", "y"),
		model.Int64("a", 2),
	}

	testCases := []struct {
		key   string
		found bool
		kv    model.KeyValue
	}{
		{"b", false, model.KeyValue{}},
		{"a", true, model.Int64("a", 2)},
		{"x", true, model.String("x", "z")},
	}

	for _, test := range testCases {
		t.Run(fmt.Sprintf("%+v", test), func(t *testing.T) {
			kv, found := input.FindByKey(test.key)
			assert.Equal(t, test.found, found)
			assert.Equal(t, test.kv, kv)
		})
	}
}

func TestKeyValuesEqual(t *testing.T) {
	v1 := model.String("s", "abc")
	v2 := model.Int64("i", 123)
	v3 := model.Float64("f", 123.4)
	assert.True(t, model.KeyValues{}.Equal(model.KeyValues{}))
	assert.True(t, model.KeyValues{v1}.Equal(model.KeyValues{v1}))
	assert.True(t, model.KeyValues{v1, v2}.Equal(model.KeyValues{v1, v2}))
	assert.False(t, model.KeyValues{v1, v2}.Equal(model.KeyValues{v2, v1}))
	assert.False(t, model.KeyValues{v1, v2}.Equal(model.KeyValues{v1, v3}))
	assert.False(t, model.KeyValues{v1, v2}.Equal(model.KeyValues{v1, v2, v3}))
}

func TestKeyValuesHashErrors(t *testing.T) {
	kvs := model.KeyValues{
		model.String("x", "y"),
	}
	someErr := errors.New("some error")
	for i := 0; i < 3; i++ {
		// mock writer with i-1 successful answers, and one failed answer
		w := &mockHashWwiter{
			answers: make([]mockHashWwiterAnswer, i+1),
		}
		w.answers[i] = mockHashWwiterAnswer{1, someErr}
		assert.Equal(t, someErr, kvs.Hash(w))
	}
	kvs[0].VType = model.ValueType(-1)
	w := &mockHashWwiter{
		answers: make([]mockHashWwiterAnswer, 3),
	}
	assert.EqualError(t, kvs.Hash(w), "unknown type -1")
}

// No memory allocations for IsLess and Equal
// 18.6 ns/op	       0 B/op	       0 allocs/op
func BenchmarkKeyValueIsLessAndEquals(b *testing.B) {
	v1 := model.KeyValue{Key: "x", VType: model.ValueType(-1)}
	v2 := model.KeyValue{Key: "x", VType: model.ValueType(-1)}
	for i := 0; i < b.N; i++ {
		v1.IsLess(&v2)
		v2.IsLess(&v1)
		v1.Equal(&v2)
		v2.Equal(&v1)
	}
}

// No memory allocations for Sort (1 alloc comes from the algorithm, not from comparisons)
// 107 ns/op	      32 B/op	       1 allocs/op
func BenchmarkKeyValuesSort(b *testing.B) {
	v1 := model.KeyValue{Key: "x", VType: model.ValueType(-1)}
	v2 := model.KeyValue{Key: "x", VType: model.ValueType(-1)}
	list := model.KeyValues(make([]model.KeyValue, 5))
	for i := 0; i < b.N; i++ {
		list[0], list[1], list[2], list[3], list[4] = v2, v1, v2, v1, v2
		list.Sort()
	}
}
