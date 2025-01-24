// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package jaeger

import (
	"testing"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/stretchr/testify/assert"
)

func TestFromDomainOneSpan(t *testing.T) {
	spanFile := "fixtures/domain_01.json"
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
	file := "fixtures/domain_03.json"
	modelSpans := loadSpans(t, file)

	batchFile := "fixtures/thrift_batch_01.json"
	jaegerBatch := loadBatch(t, batchFile)

	jaegerSpans := FromDomain(modelSpans)
	newModelSpans := ToDomain(jaegerSpans, jaegerBatch.Process)
	for i := range newModelSpans {
		modelSpan := modelSpans[i]
		newModelSpan := newModelSpans[i]
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
