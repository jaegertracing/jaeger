// Copyright (c) 2021 The Jaeger Authors.
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

package app

import (
	"time"

	"github.com/jaegertracing/jaeger/pkg/es/client"
)

type AliasFilter struct {
	Indices []client.Index
}

func (af AliasFilter) HasAliasEmpty(aliasName string) bool {
	aliases := af.FilterByAlias([]string{aliasName})
	return len(aliases) == 0
}

func (af AliasFilter) FilterByAlias(aliases []string) []client.Index {
	return af.filterByAliasWithOptions(aliases, false)
}

func (af AliasFilter) FilterByAliasExclude(aliases []string) []client.Index {
	return af.filterByAliasWithOptions(aliases, true)
}

func (af AliasFilter) filterByAliasWithOptions(aliases []string, exclude bool) []client.Index {
	var results []client.Index
	for _, alias := range aliases {
		for _, index := range af.Indices {
			hasAlias := index.Aliases[alias]
			if !exclude {
				if hasAlias {
					results = append(results, index)
				}
			} else {
				if !hasAlias {
					results = append(results, index)
				}
			}

		}
	}
	return results
}

func (i *AliasFilter) FilterByDate(DeleteBeforeThisDate time.Time) []client.Index {
	var filtered []client.Index
	for _, in := range i.Indices {
		if in.CreationTime.Before(DeleteBeforeThisDate) {
			filtered = append(filtered, in)
		}
	}
	return filtered
}
