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

package elasticsearchexporter

import "time"

func newIndexNameProvider(index, prefix string, useAliases bool, useArchive bool) indexNameProvider {
	if prefix != "" {
		prefix = prefix + "-"
		index = prefix + index
	}
	index = index + "-"
	if useArchive {
		index = index + "archive"
	}
	if useAliases {
		if index[len(index)-1] != '-' {
			index += "-"
		}
		index = index + "write"
	}
	return indexNameProvider{
		index:          index,
		useSingleIndex: useAliases || useArchive,
	}
}

type indexNameProvider struct {
	index          string
	useSingleIndex bool
}

func (n indexNameProvider) get(date time.Time) string {
	if n.useSingleIndex {
		return n.index
	}
	spanDate := date.UTC().Format(indexDateFormat)
	return n.index + spanDate
}
