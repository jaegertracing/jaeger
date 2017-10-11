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

import "github.com/uber/jaeger/model"

// GetAllUniqueTags creates a list of all unique tags from a set of filtered tags.
func GetAllUniqueTags(span *model.Span, tagFilter TagFilter) []TagInsertion {
	tags := tagFilter.FilterProcessTags(span.Process.Tags)
	tags = append(tags, tagFilter.FilterTags(span.Tags)...)
	for _, log := range span.Logs {
		tags = append(tags, tagFilter.FilterLogFields(log.Fields)...)
	}
	tags.Sort()
	uniqueTags := make([]TagInsertion, 0, len(tags))
	for i := range tags {
		if tags[i].VType == model.BinaryType {
			continue // do not index binary tags
		}
		if i > 0 && tags[i-1].Equal(&tags[i]) {
			continue // skip identical tags
		}
		uniqueTags = append(uniqueTags, TagInsertion{
			ServiceName: span.Process.ServiceName,
			TagKey:      tags[i].Key,
			TagValue:    tags[i].AsString(),
		})
	}
	return uniqueTags
}
