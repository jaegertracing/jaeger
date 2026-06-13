// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultIndexFilter(t *testing.T) {
	span := &Span{}
	filter := DefaultIndexFilter
	assert.True(t, filter(span, DurationIndex))
	assert.True(t, filter(span, ServiceIndex))
	assert.True(t, filter(span, OperationIndex))
}
