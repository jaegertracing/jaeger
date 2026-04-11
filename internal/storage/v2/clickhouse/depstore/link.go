// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package depstore

import "github.com/jaegertracing/jaeger-idl/model/v1"

// dependencyLink is the JSON representation of a single dependency link
// as stored in ClickHouse.
type dependencyLink struct {
	Parent    string `json:"parent"`
	Child     string `json:"child"`
	CallCount uint64 `json:"callCount"`
}

func dependencyLinkFromModel(d model.DependencyLink) dependencyLink {
	return dependencyLink{
		Parent:    d.Parent,
		Child:     d.Child,
		CallCount: d.CallCount,
	}
}

func (l dependencyLink) toModel() model.DependencyLink {
	return model.DependencyLink{
		Parent:    l.Parent,
		Child:     l.Child,
		CallCount: l.CallCount,
	}
}

// dependencyLinks represents the JSON value stored in the `dependencies_json`
// column of the ClickHouse dependencies table. All dependency links computed
// in a single aggregation interval are stored as one JSON blob per row rather
// than one row per link. This keeps the table compact and allows an entire
// snapshot to be read or written in a single operation.
type dependencyLinks []dependencyLink

func dependencyLinksFromModel(deps []model.DependencyLink) dependencyLinks {
	if deps == nil {
		return nil
	}
	links := make(dependencyLinks, len(deps))
	for i, d := range deps {
		links[i] = dependencyLinkFromModel(d)
	}
	return links
}

func (links dependencyLinks) toModel() []model.DependencyLink {
	if links == nil {
		return nil
	}
	result := make([]model.DependencyLink, len(links))
	for i, l := range links {
		result[i] = l.toModel()
	}
	return result
}
