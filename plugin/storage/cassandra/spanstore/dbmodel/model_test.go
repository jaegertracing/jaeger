// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"testing"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/stretchr/testify/assert"
)

func TestTagInsertionString(t *testing.T) {
	v := TagInsertion{"x", "y", "z"}
	assert.Equal(t, "x:y:z", v.String())
}

func TestTraceIDString(t *testing.T) {
	id := TraceIDFromDomain(model.NewTraceID(1, 1))
	assert.Equal(t, "00000000000000010000000000000001", id.String())
}
