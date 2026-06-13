// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

// TagFilterDropAll filters all fields of a given type.
type TagFilterDropAll struct {
	dropTags        bool
	dropProcessTags bool
	dropLogs        bool
}

// NewTagFilterDropAll return a filter that filters all of the specified type
func NewTagFilterDropAll(dropTags bool, dropProcessTags bool, dropLogs bool) *TagFilterDropAll {
	return &TagFilterDropAll{
		dropTags:        dropTags,
		dropProcessTags: dropProcessTags,
		dropLogs:        dropLogs,
	}
}

// FilterProcessTags implements TagFilter
func (f *TagFilterDropAll) FilterProcessTags(_ *Span, processTags []KeyValue) []KeyValue {
	if f.dropProcessTags {
		return []KeyValue{}
	}
	return processTags
}

// FilterTags implements TagFilter
func (f *TagFilterDropAll) FilterTags(_ *Span, tags []KeyValue) []KeyValue {
	if f.dropTags {
		return []KeyValue{}
	}
	return tags
}

// FilterLogFields implements TagFilter
func (f *TagFilterDropAll) FilterLogFields(_ *Span, logFields []KeyValue) []KeyValue {
	if f.dropLogs {
		return []KeyValue{}
	}
	return logFields
}
