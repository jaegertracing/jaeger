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

import (
	"io"
	"text/template"
)

// TemplateApplier is an abstraction to support mocking text/template
type TemplateApplier interface {
	Execute(wr io.Writer, data interface{}) error
}

// TemplateBuilder is an abstraction to support mocking text/template
type TemplateBuilder interface {
	Parse(text string) (TemplateApplier, error)
}

// TextTemplateBuilder implements TemplateBuilder
type TextTemplateBuilder struct{}

// Parse is a wrapper for template.Parse
func (t TextTemplateBuilder) Parse(mapping string) (TemplateApplier, error) {
	return template.New("mapping").Parse(mapping)
}

// NewTextTemplateBuilder returns a TextTemplateBuilder
func NewTextTemplateBuilder() TemplateBuilder {
	return TextTemplateBuilder{}
}
