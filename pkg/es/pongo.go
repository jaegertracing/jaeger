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

package es

import "github.com/flosch/pongo2/v4"

//TemplateApplier is an abstraction to support mocking pongo2
type TemplateApplier interface {
	Execute(ctx pongo2.Context) (string, error)
}

//TemplateBuilder is an abstraction to support mocking pongo2
type TemplateBuilder interface {
	FromString(mapping string) (TemplateApplier, error)
}

//PongoTemplateBuilder implements TemplateBuilder
type PongoTemplateBuilder struct{}

// FromString is a wrapper for pongo2.FromString
func (p PongoTemplateBuilder) FromString(mapping string) (TemplateApplier, error) {
	return pongo2.FromString(mapping)
}
