// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package zipkin

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger-idl/model/v1"
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
	assert.Equal(t, p1, ht.add(p1))
	assert.Equal(t, p2, ht.add(p2))
	assert.Equal(t, p1, ht.add(p1))
	assert.Equal(t, p2, ht.add(p2))
	assert.Equal(t, p1, ht.add(p1dup))
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
	assert.Equal(t, p1, ht.add(p1))
	assert.Equal(t, p2, ht.add(p2))
	assert.Equal(t, p1, ht.add(p1))
	assert.Equal(t, p2, ht.add(p2))
}
