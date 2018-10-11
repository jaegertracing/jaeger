// Copyright (c) 2018 The Jaeger Authors.
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

import "github.com/jaegertracing/jaeger/model"

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
		}
	}
	return ret
}

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
		}
	}
	return ret
}
