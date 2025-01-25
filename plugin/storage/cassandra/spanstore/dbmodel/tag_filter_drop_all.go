// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"github.com/jaegertracing/jaeger-idl/model/v1"
)

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
func (f *TagFilterDropAll) FilterProcessTags(_ *model.Span, processTags model.KeyValues) model.KeyValues {
	if f.dropProcessTags {
		return model.KeyValues{}
	}
	return processTags
}

// FilterTags implements TagFilter
func (f *TagFilterDropAll) FilterTags(_ *model.Span, tags model.KeyValues) model.KeyValues {
	if f.dropTags {
		return model.KeyValues{}
	}
	return tags
}

// FilterLogFields implements TagFilter
func (f *TagFilterDropAll) FilterLogFields(_ *model.Span, logFields model.KeyValues) model.KeyValues {
	if f.dropLogs {
		return model.KeyValues{}
	}
	return logFields
}
