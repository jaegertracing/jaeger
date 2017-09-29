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

// DefaultTagFilter returns a filter that retrieves all tags from span.Tags, span.Logs, and span.Process.
func DefaultTagFilter() FilterTags {
	return filterNothing
}

func filterNothing(span *model.Span) []TagInsertion {
	process := span.Process
	allTags := span.Tags
	allTags = append(allTags, process.Tags...)
	for _, log := range span.Logs {
		allTags = append(allTags, log.Fields...)
	}
	return getUniqueTags(process.ServiceName, allTags)
}
