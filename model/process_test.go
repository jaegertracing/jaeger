// Copyright (c) 2017 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package model_test

import (
	"errors"
	"hash/fnv"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/model"
)

func TestProcessEqual(t *testing.T) {
	p1 := model.NewProcess("s1", []model.KeyValue{
		model.Int64("a", 1),
		model.String("x", "y"),
	})
	p2 := model.NewProcess("s1", []model.KeyValue{
		model.Int64("a", 1),
		model.String("x", "y"),
	})
	p3 := model.NewProcess("S2", []model.KeyValue{
		model.Int64("a", 1),
		model.String("x", "y"),
	})
	p4 := model.NewProcess("s1", []model.KeyValue{
		model.Int64("a", 1),
		model.Float64("a", 1.1),
		model.String("x", "y"),
	})
	p5 := model.NewProcess("s1", []model.KeyValue{
		model.Float64("a", 1.1),
		model.String("x", "y"),
	})
	assert.Equal(t, p1, p2)
	assert.True(t, p1.Equal(p2))
	assert.False(t, p1.Equal(p3))
	assert.False(t, p1.Equal(p4))
	assert.False(t, p1.Equal(p5))
}

func Hash(w io.Writer) {
	w.Write([]byte("hello"))
}

func TestX(t *testing.T) {
	h := fnv.New64a()
	Hash(h)
}

func TestProcessHash(t *testing.T) {
	p1 := model.NewProcess("s1", []model.KeyValue{
		model.String("x", "y"),
		model.Int64("y", 1),
		model.Binary("z", []byte{1}),
	})
	p1copy := model.NewProcess("s1", []model.KeyValue{
		model.String("x", "y"),
		model.Int64("y", 1),
		model.Binary("z", []byte{1}),
	})
	p2 := model.NewProcess("s2", []model.KeyValue{
		model.String("x", "y"),
		model.Int64("y", 1),
		model.Binary("z", []byte{1}),
	})
	p1h, err := model.HashCode(p1)
	require.NoError(t, err)
	p1ch, err := model.HashCode(p1copy)
	require.NoError(t, err)
	p2h, err := model.HashCode(p2)
	require.NoError(t, err)
	assert.Equal(t, p1h, p1ch)
	assert.NotEqual(t, p1h, p2h)
}

func TestProcessHashError(t *testing.T) {
	p1 := model.NewProcess("s1", []model.KeyValue{
		model.String("x", "y"),
	})
	someErr := errors.New("some error")
	w := &mockHashWwiter{
		answers: []mockHashWwiterAnswer{
			{1, someErr},
		},
	}
	assert.Equal(t, someErr, p1.Hash(w))
}
