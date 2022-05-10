// Copyright (c) 2022 The Jaeger Authors.
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

package sanitizer

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/model"
)

func TestNewStandardSanitizers(t *testing.T) {
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
