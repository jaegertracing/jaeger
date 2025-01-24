// Copyright (c) 2022 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package sanitizer

import (
	"testing"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/stretchr/testify/assert"
)

func TestNewStandardSanitizers(*testing.T) {
	NewStandardSanitizers()
}

func TestChainedSanitizer(t *testing.T) {
	var s1 SanitizeSpan = func(span *model.Span) *model.Span {
		span.Process = &model.Process{ServiceName: "s1"}
		return span
	}
	var s2 SanitizeSpan = func(span *model.Span) *model.Span {
		span.Process = &model.Process{ServiceName: "s2"}
		return span
	}
	c1 := NewChainedSanitizer(s1)
	sp1 := c1(&model.Span{})
	assert.Equal(t, "s1", sp1.Process.ServiceName)
	c2 := NewChainedSanitizer(s1, s2)
	sp2 := c2(&model.Span{})
	assert.Equal(t, "s2", sp2.Process.ServiceName)
}
