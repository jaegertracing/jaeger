// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"testing"

	"github.com/kr/pretty"
	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger-idl/model/v1"
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

func getTestSpan() *Span {
	span := &Span{
		TraceID:       someDBTraceID,
		SpanID:        someSpanID,
		OperationName: someOperationName,
		Flags:         someFlags,
		StartTime:     int64(model.TimeAsEpochMicroseconds(someStartTime)),
		Duration:      int64(model.DurationAsMicroseconds(someDuration)),
		Tags:          someDBTags,
		Logs:          someDBLogs,
		Refs:          someDBRefs,
		Process:       someDBProcess,
		ServiceName:   someServiceName,
	}
	// there is no way to validate if the hash code is "correct" or not,
	// other than comparing it with some magic number that keeps changing
	// as the model changes. So let's just make sure the code is being
	// calculated during the conversion.
	spanHash, _ := model.HashCode(span)
	span.SpanHash = int64(spanHash)
	return span
}
