// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"testing"

	"github.com/kr/pretty"
	"github.com/stretchr/testify/assert"
)

func TestDefaultTagFilter(t *testing.T) {
	span := getTestSpan()
	expectedTags := append(append(someDBTags, someDBTags...), someDBTags...)
	filteredTags := DefaultTagFilter.FilterProcessTags(span, span.Process.Tags)
	filteredTags = append(filteredTags, DefaultTagFilter.FilterTags(span, span.Tags)...)
	for _, log := range span.Logs {
		filteredTags = append(filteredTags, DefaultTagFilter.FilterLogFields(span, log.Fields)...)
	}
	compareTags(t, expectedTags, filteredTags)
}

type onlyStringsFilter struct{}

func (onlyStringsFilter) filterStringTags(tags []KeyValue) []KeyValue {
	var ret []KeyValue
	for _, tag := range tags {
		if tag.ValueType == StringType {
			ret = append(ret, tag)
		}
	}
	return ret
}

func (f onlyStringsFilter) FilterProcessTags(_ *Span, processTags []KeyValue) []KeyValue {
	return f.filterStringTags(processTags)
}

func (f onlyStringsFilter) FilterTags(_ *Span, tags []KeyValue) []KeyValue {
	return f.filterStringTags(tags)
}

func (f onlyStringsFilter) FilterLogFields(_ *Span, logFields []KeyValue) []KeyValue {
	return f.filterStringTags(logFields)
}

func TestChainedTagFilter(t *testing.T) {
	expectedTags := []KeyValue{{Key: someStringTagKey, ValueType: StringType, ValueString: someStringTagValue}}
	filter := NewChainedTagFilter(DefaultTagFilter, onlyStringsFilter{})
	filteredTags := filter.FilterProcessTags(nil, someDBTags)
	compareTags(t, expectedTags, filteredTags)
	filteredTags = filter.FilterTags(nil, someDBTags)
	compareTags(t, expectedTags, filteredTags)
	filteredTags = filter.FilterLogFields(nil, someDBTags)
	compareTags(t, expectedTags, filteredTags)
}

func compareTags(t *testing.T, expected, actual []KeyValue) {
	if !assert.Equal(t, expected, actual) {
		for _, diff := range pretty.Diff(expected, actual) {
			t.Log(diff)
		}
	}
}
