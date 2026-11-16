// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import "github.com/jaegertracing/jaeger-idl/model/v1"

// FromDomainDependencies converts model dependencies to database representation
func FromDomainDependencies(dLinks []model.DependencyLink) []DependencyLink {
	if dLinks == nil {
		return nil
	}
	ret := make([]DependencyLink, len(dLinks))
	for i, d := range dLinks {
		ret[i] = DependencyLink{
			CallCount: d.CallCount,
			Parent:    d.Parent,
			Child:     d.Child,
			Source:    d.Source,
		}
	}
	return ret
}

// ToDomainDependencies converts database representation of dependencies to model.
// A link stored without a source is returned as a link from Jaeger's own traces,
// because earlier versions stored no source and an empty source is omitted on write.
func ToDomainDependencies(dLinks []DependencyLink) []model.DependencyLink {
	if dLinks == nil {
		return nil
	}
	ret := make([]model.DependencyLink, len(dLinks))
	for i, d := range dLinks {
		ret[i] = model.DependencyLink{
			CallCount: d.CallCount,
			Parent:    d.Parent,
			Child:     d.Child,
			Source:    d.Source,
		}.ApplyDefaults()
	}
	return ret
}
