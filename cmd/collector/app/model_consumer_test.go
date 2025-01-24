// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger-idl/model/v1"
)

func TestChainedProcessSpan(t *testing.T) {
	happened1 := false
	happened2 := false
	func1 := func(_ *model.Span, _ /* tenant */ string) { happened1 = true }
	func2 := func(_ *model.Span, _ /* tenant */ string) { happened2 = true }
	chained := ChainedProcessSpan(func1, func2)
	chained(&model.Span{}, "")
	assert.True(t, happened1)
	assert.True(t, happened2)
}
