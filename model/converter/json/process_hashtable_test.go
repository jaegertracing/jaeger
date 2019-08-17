// Copyright (c) 2019 The Jaeger Authors.
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

package json

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/model"
)

func TestProcessHashtable(t *testing.T) {
	ht := &processHashtable{}

	p1 := model.NewProcess("s1", []model.KeyValue{
		model.String("ip", "1.2.3.4"),
		model.String("host", "google.com"),
	})
	// same process but with different order of tags
	p1dup := model.NewProcess("s1", []model.KeyValue{
		model.String("host", "google.com"),
		model.String("ip", "1.2.3.4"),
	})
	p2 := model.NewProcess("s2", []model.KeyValue{
		model.String("host", "facebook.com"),
	})

	assert.Equal(t, "p1", ht.getKey(p1))
	assert.Equal(t, "p1", ht.getKey(p1))
	assert.Equal(t, "p1", ht.getKey(p1dup))
	assert.Equal(t, "p2", ht.getKey(p2))

	expectedMapping := map[string]*model.Process{
		"p1": p1,
		"p2": p2,
	}
	assert.Equal(t, expectedMapping, ht.getMapping())
}

func TestProcessHashtableCollision(t *testing.T) {
	ht := &processHashtable{}
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
	assert.Equal(t, "p1", ht.getKey(p1))
	assert.Equal(t, "p2", ht.getKey(p2))
}
