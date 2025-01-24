// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package json

import (
	"testing"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/stretchr/testify/assert"
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
