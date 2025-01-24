// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dependencystore

import (
	"testing"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/stretchr/testify/assert"
)

func TestSeekToSpan(t *testing.T) {
	span := seekToSpan(&model.Trace{}, model.SpanID(uint64(1)))
	assert.Nil(t, span)
}
