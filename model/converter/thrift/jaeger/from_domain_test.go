// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package jaeger

import (
	"testing"

	"github.com/stretchr/testify/assert"

	j "github.com/uber/jaeger/thrift-gen/jaeger"
)

const (
	millisecondsConversion = 1000
)

func spanRefsEqual(refs []*j.SpanRef, otherRefs []*j.SpanRef) bool {
	if len(refs) != len(otherRefs) {
		return false
	}

	for idx, ref := range refs {
		if *ref != *otherRefs[idx] {
			return false
		}
	}
	return true
}

func TestFromDomainSpan(t *testing.T) {
	spanFile := "fixtures/model_01.json"
	mSpans := loadSpans(t, spanFile)

	batchFile := "fixtures/thrift_batch_01.json"
	jaegerBatch := loadBatch(t, batchFile)

	modelSpan := mSpans[0]
	jaegerSpan := FromDomainSpan(modelSpan)
	newModelSpan := ToDomainSpan(jaegerSpan, jaegerBatch.Process)

	assert.Equal(t, modelSpan, newModelSpan)
}

func TestFromDomain(t *testing.T) {
	file := "fixtures/model_03.json"
	mSpans := loadSpans(t, file)

	batchFile := "fixtures/thrift_batch_01.json"
	jaegerBatch := loadBatch(t, batchFile)

	jaegerSpans := FromDomain(mSpans)
	newModelSpans := ToDomain(jaegerSpans, jaegerBatch.Process)
	assert.Equal(t, mSpans, newModelSpans)
}
