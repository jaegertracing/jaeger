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

package zipkin

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/model"
)

func TestProcessHashtable(t *testing.T) {
	p1 := model.NewProcess("s1", []model.KeyValue{
		model.String("host", "google.com"),
	})
	p1dup := model.NewProcess("s1", []model.KeyValue{
		model.String("host", "google.com"),
	})
	p2 := model.NewProcess("s2", []model.KeyValue{
		model.String("host", "facebook.com"),
	})
	ht := newProcessHashtable()
	assert.True(t, p1 == ht.add(p1))
	assert.True(t, p2 == ht.add(p2))
	assert.True(t, p1 == ht.add(p1))
	assert.True(t, p2 == ht.add(p2))
	assert.True(t, p1 == ht.add(p1dup))
}

func TestProcessHashtableCollision(t *testing.T) {
	ht := newProcessHashtable()
	// hash all processes to the same number
	ht.extHash = func(*model.Process) uint64 {
		return 42
	}

	p1 := model.NewProcess("s1", []model.KeyValue{
		model.String("host", "google.com"),
	})
	p2 := model.NewProcess("s2", []model.KeyValue{
		model.String("host", "facebook.com"),
	})
	assert.True(t, p1 == ht.add(p1))
	assert.True(t, p2 == ht.add(p2))
	assert.True(t, p1 == ht.add(p1))
	assert.True(t, p2 == ht.add(p2))
}
