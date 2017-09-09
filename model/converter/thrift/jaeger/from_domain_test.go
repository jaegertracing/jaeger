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

package jaeger

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/uber/jaeger/model"
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
	modelSpans := loadSpans(t, spanFile)

	batchFile := "fixtures/thrift_batch_01.json"
	jaegerBatch := loadBatch(t, batchFile)

	modelSpan := modelSpans[0]
	jaegerSpan := FromDomainSpan(modelSpan)
	newModelSpan := ToDomainSpan(jaegerSpan, jaegerBatch.Process)

	modelSpan.NormalizeTimestamps()
	newModelSpan.NormalizeTimestamps()
	assert.Equal(t, modelSpan, newModelSpan)
}

func TestFromDomain(t *testing.T) {
	file := "fixtures/model_03.json"
	modelSpans := loadSpans(t, file)

	batchFile := "fixtures/thrift_batch_01.json"
	jaegerBatch := loadBatch(t, batchFile)

	jaegerSpans := FromDomain(modelSpans)
	newModelSpans := ToDomain(jaegerSpans, jaegerBatch.Process)
	for idx := range newModelSpans {
		modelSpan := modelSpans[idx]
		newModelSpan := newModelSpans[idx]
		modelSpan.NormalizeTimestamps()
		newModelSpan.NormalizeTimestamps()
	}
	assert.Equal(t, modelSpans, newModelSpans)
}

func TestKeyValueToTag(t *testing.T) {
	dToJ := domainToJaegerTransformer{}
	jaegerTag := dToJ.keyValueToTag(&model.KeyValue{
		Key:   "some-error",
		VType: model.ValueType(-1),
	})

	assert.Equal(t, "Error", jaegerTag.Key)
	assert.Equal(t, "No suitable tag type found for: -1", *jaegerTag.VStr)
}
