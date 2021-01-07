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

package esutil

import "time"

// Alias is used to configure the kind of index alias
type Alias string

const (
	// AliasNone configures no aliasing
	AliasNone Alias = "none"
	// AliasRead configures read aliases
	AliasRead = "read"
	// AliasWrite configures write aliases
	AliasWrite = "write"
)

// IndexNameProvider creates standard index names from dates
type IndexNameProvider struct {
	index          string
	dateLayout     string
	useSingleIndex bool
}

// NewIndexNameProvider constructs a new IndexNameProvider
func NewIndexNameProvider(index, prefix, layout string, alias Alias, archive bool) IndexNameProvider {
	if prefix != "" {
		index = prefix + "-" + index
	}
	index += "-"
	if archive {
		index += "archive"
	}
	if alias != AliasNone {
		if index[len(index)-1] != '-' {
			index += "-"
		}
		index += string(alias)
	}
	return IndexNameProvider{
		index:          index,
		dateLayout:     layout,
		useSingleIndex: archive || (alias != AliasNone),
	}
}

// IndexNameRange returns a slice of index names.  One for every date in the range.
func (n IndexNameProvider) IndexNameRange(start, end time.Time) []string {
	if n.useSingleIndex {
		return []string{n.index}
	}
	var indices []string
	firstIndex := n.index + start.UTC().Format(n.dateLayout)
	currentIndex := n.index + end.UTC().Format(n.dateLayout)
	for currentIndex != firstIndex {
		indices = append(indices, currentIndex)
		end = end.Add(-24 * time.Hour)
		currentIndex = n.index + end.UTC().Format(n.dateLayout)
	}
	indices = append(indices, firstIndex)
	return indices
}

// IndexName returns a single index name for the provided date.
func (n IndexNameProvider) IndexName(date time.Time) string {
	if n.useSingleIndex {
		return n.index
	}
	spanDate := date.UTC().Format(n.dateLayout)
	return n.index + spanDate
}
