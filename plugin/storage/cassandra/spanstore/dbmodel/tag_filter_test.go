// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"testing"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/kr/pretty"
	"github.com/stretchr/testify/assert"
)

func TestDefaultTagFilter(t *testing.T) {
	span := getTestJaegerSpan()
	expectedTags := append(append(someTags, someTags...), someTags...)
	filteredTags := DefaultTagFilter.FilterProcessTags(span, span.Process.Tags)
	filteredTags = append(filteredTags, DefaultTagFilter.FilterTags(span, span.Tags)...)
	for _, log := range span.Logs {
		filteredTags = append(filteredTags, DefaultTagFilter.FilterLogFields(span, log.Fields)...)
	}
	compareTags(t, expectedTags, filteredTags)
}

type onlyStringsFilter struct{}

func (onlyStringsFilter) filterStringTags(tags model.KeyValues) model.KeyValues {
	var ret model.KeyValues
	for _, tag := range tags {
		if tag.VType == model.StringType {
			ret = append(ret, tag)
		}
	}
	return ret
}

func (f onlyStringsFilter) FilterProcessTags(_ *model.Span, processTags model.KeyValues) model.KeyValues {
	return f.filterStringTags(processTags)
}

func (f onlyStringsFilter) FilterTags(_ *model.Span, tags model.KeyValues) model.KeyValues {
	return f.filterStringTags(tags)
}

func (f onlyStringsFilter) FilterLogFields(_ *model.Span, logFields model.KeyValues) model.KeyValues {
	return f.filterStringTags(logFields)
}

func TestChainedTagFilter(t *testing.T) {
	expectedTags := model.KeyValues{model.String(someStringTagKey, someStringTagValue)}
	filter := NewChainedTagFilter(DefaultTagFilter, onlyStringsFilter{})
	filteredTags := filter.FilterProcessTags(nil, someTags)
	compareTags(t, expectedTags, filteredTags)
	filteredTags = filter.FilterTags(nil, someTags)
	compareTags(t, expectedTags, filteredTags)
	filteredTags = filter.FilterLogFields(nil, someTags)
	compareTags(t, expectedTags, filteredTags)
}

func compareTags(t *testing.T, expected, actual model.KeyValues) {
	if !assert.EqualValues(t, expected, actual) {
		for _, diff := range pretty.Diff(expected, actual) {
			t.Log(diff)
		}
	}
}
