// Copyright (c) 2019 The Jaeger Authors.
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

package dbmodel

import (
	"github.com/jaegertracing/jaeger/model"
)

// GetAllUniqueTags creates a list of all unique tags from a set of filtered tags.
func GetAllUniqueTags(span *model.Span, tagFilter TagFilter) []TagInsertion {
	allTags := append(model.KeyValues{}, tagFilter.FilterProcessTags(span, span.Process.Tags)...)
	allTags = append(allTags, tagFilter.FilterTags(span, span.Tags)...)
	for _, log := range span.Logs {
		allTags = append(allTags, tagFilter.FilterLogFields(span, log.Fields)...)
	}
	allTags.Sort()
	uniqueTags := make([]TagInsertion, 0, len(allTags))
	for i := range allTags {
		if allTags[i].VType == model.BinaryType {
			continue // do not index binary tags
		}
		if i > 0 && allTags[i-1].Equal(&allTags[i]) {
			continue // skip identical tags
		}
		uniqueTags = append(uniqueTags, TagInsertion{
			ServiceName: span.Process.ServiceName,
			TagKey:      allTags[i].Key,
			TagValue:    allTags[i].AsStringLossy(),
		})
	}
	return uniqueTags
}
