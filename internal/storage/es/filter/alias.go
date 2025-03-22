// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package filter

import (
	"github.com/jaegertracing/jaeger/internal/storage/es/client"
)

// AliasExists check if the some of indices has a certain alias name
func AliasExists(indices []client.Index, aliasName string) bool {
	aliases := ByAlias(indices, []string{aliasName})
	return len(aliases) > 0
}

// ByAlias filter indices that have an alias in the array of alias
func ByAlias(indices []client.Index, aliases []string) []client.Index {
	return filterByAliasWithOptions(indices, aliases, false)
}

// ByAliasExclude filter indices that doesn't have an alias in the array of alias
func ByAliasExclude(indices []client.Index, aliases []string) []client.Index {
	return filterByAliasWithOptions(indices, aliases, true)
}

func filterByAliasWithOptions(indices []client.Index, aliases []string, exclude bool) []client.Index {
	var results []client.Index
	for _, alias := range aliases {
		for _, index := range indices {
			hasAlias := index.Aliases[alias]
			if hasAlias {
				results = append(results, index)
			}
		}
	}
	if exclude {
		return exlude(indices, results)
	}
	return results
}

func exlude(indices []client.Index, exclusionList []client.Index) []client.Index {
	excludedIndices := make([]client.Index, 0, len(indices))
	for _, idx := range indices {
		if !contains(idx, exclusionList) {
			excludedIndices = append(excludedIndices, idx)
		}
	}
	return excludedIndices
}

func contains(index client.Index, indexList []client.Index) bool {
	for _, idx := range indexList {
		if idx.Index == index.Index {
			return true
		}
	}
	return false
}
