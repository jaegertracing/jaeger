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

package client

type IndexAPI interface {
	GetJaegerIndices(prefix string) ([]Index, error)
	DeleteIndices(indices []Index) error
	CreateIndex(index string) error
	CreateAlias(aliases []Alias) error
	DeleteAlias(aliases []Alias) error
	CreateTemplate(template, name string) error
	Rollover(rolloverTarget string, conditions map[string]any) error
}

type ClusterAPI interface {
	Version() (uint, error)
}

type IndexManagementLifecycleAPI interface {
	Exists(name string) (bool, error)
}
