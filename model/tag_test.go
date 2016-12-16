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
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/uber/jaeger/model"
)

func TestSort(t *testing.T) {
	input := model.Tags{
		model.Tag(model.String("x", "z")),
		model.Tag(model.String("x", "y")),
		model.Tag(model.Int64("a", 2)),
		model.Tag(model.Int64("a", 1)),
		model.Tag(model.Float64("x", 2)),
		model.Tag(model.Float64("x", 1)),
		model.Tag(model.Bool("x", true)),
		model.Tag(model.Bool("x", false)),
	}
	expected := model.Tags{
		model.Tag(model.Int64("a", 1)),
		model.Tag(model.Int64("a", 2)),
		model.Tag(model.String("x", "y")),
		model.Tag(model.String("x", "z")),
		model.Tag(model.Bool("x", false)),
		model.Tag(model.Bool("x", true)),
		model.Tag(model.Float64("x", 1)),
		model.Tag(model.Float64("x", 2)),
	}
	input.Sort()
	assert.Equal(t, expected, input)
}

func TestFindByKey(t *testing.T) {
	input := model.Tags{
		model.Tag(model.String("x", "z")),
		model.Tag(model.String("x", "y")),
		model.Tag(model.Int64("a", 2)),
	}

	testCases := []struct {
		key   string
		found bool
		tag   model.Tag
	}{
		{"b", false, model.Tag{}},
		{"a", true, model.Tag(model.Int64("a", 2))},
		{"x", true, model.Tag(model.String("x", "z"))},
	}

	for _, test := range testCases {
		t.Run(fmt.Sprintf("%+v", test), func(t *testing.T) {
			tag, found := input.FindByKey(test.key)
			assert.Equal(t, test.found, found)
			assert.Equal(t, test.tag, tag)
		})
	}
}
