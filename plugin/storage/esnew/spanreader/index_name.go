// Copyright (c) 2020 The Jaeger Authors.
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

package spanreader

import "time"

const indexDateFormat = "2006-01-02" // date format for index e.g. 2020-01-20

type indexNameProvider struct {
	index       string
	noDateIndex bool
}

func newIndexNameProvider(index, prefix string, useAliases bool, archive bool) indexNameProvider {
	if prefix != "" {
		index = prefix + "-" + index
	}
	index += "-"
	if archive {
		index += "archive"
	}
	if useAliases {
		if index[len(index)-1] != '-' {
			index += "-"
		}
		index += "read"
	}
	return indexNameProvider{
		index:       index,
		noDateIndex: archive || useAliases,
	}
}

func (n indexNameProvider) get(start, end time.Time) []string {
	if n.noDateIndex {
		return []string{n.index}
	}
	var indices []string
	firstIndex := n.index + start.UTC().Format(indexDateFormat)
	currentIndex := n.index + end.UTC().Format(indexDateFormat)
	for currentIndex != firstIndex {
		indices = append(indices, currentIndex)
		end = end.Add(-24 * time.Hour)
		currentIndex = n.index + end.UTC().Format(indexDateFormat)
	}
	indices = append(indices, firstIndex)
	return indices
}
