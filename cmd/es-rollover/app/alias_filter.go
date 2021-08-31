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

import "github.com/jaegertracing/jaeger/pkg/es/client"

type AliasFilter struct {
	Indices []client.Index
}

func (af AliasFilter) HasAliasEmpty(aliasName string) bool {
	aliases := af.filterByAlias([]string{aliasName})
	return len(aliases) == 0
}

func (af AliasFilter) filterByAlias(aliases []string) []client.Index {
	var results []client.Index
	for _, alias := range aliases {
		for _, index := range af.Indices {
			hasAlias := index.Aliases[alias]
			if hasAlias {
				results = append(results, index)
			}
		}
	}
	return results
}
