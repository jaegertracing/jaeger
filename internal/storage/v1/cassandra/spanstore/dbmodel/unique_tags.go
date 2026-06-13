// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

// GetAllUniqueTags creates a list of all unique tags from a set of filtered tags.
func GetAllUniqueTags(span *Span, tagFilter TagFilter) []TagInsertion {
	allTags := append([]KeyValue{}, tagFilter.FilterProcessTags(span, span.Process.Tags)...)
	allTags = append(allTags, tagFilter.FilterTags(span, span.Tags)...)
	for _, log := range span.Logs {
		allTags = append(allTags, tagFilter.FilterLogFields(span, log.Fields)...)
	}
	SortKVs(allTags)
	uniqueTags := make([]TagInsertion, 0, len(allTags))
	for i := range allTags {
		if allTags[i].ValueType == BinaryType {
			continue // do not index binary tags
		}
		if i > 0 && allTags[i-1].Equal(&allTags[i]) {
			continue // skip identical tags
		}
		uniqueTags = append(uniqueTags, TagInsertion{
			ServiceName: span.Process.ServiceName,
			TagKey:      allTags[i].Key,
			TagValue:    allTags[i].AsString(),
		})
	}
	return uniqueTags
}
