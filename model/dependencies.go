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

package model

// DependencyLinkSource is the source of data used to generate the dependencies.
type DependencyLinkSource string

const (
	// JaegerDependencyLinkSource describes a dependency diagram that was generated from Jaeger traces.
	JaegerDependencyLinkSource = DependencyLinkSource("jaeger")
)

// DependencyLink shows dependencies between services
type DependencyLink struct {
	Parent    string               `json:"parent"`
	Child     string               `json:"child"`
	CallCount uint64               `json:"callCount"`
	Source    DependencyLinkSource `json:"source"`
}

// ApplyDefaults applies defaults to the DependencyLink.
func (d DependencyLink) ApplyDefaults() DependencyLink {
	if d.Source == "" {
		d.Source = JaegerDependencyLinkSource
	}
	return d
}
