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

// TemplateApplier applies a parsed template to input data that maps to the template's variables.
type TemplateApplier interface {
	Execute(wr io.Writer, data any) error
}

// TemplateBuilder parses a given string and returns TemplateApplier
// TemplateBuilder is an abstraction to support mocking template/text
type TemplateBuilder interface {
	Parse(text string) (TemplateApplier, error)
}

// TextTemplateBuilder implements TemplateBuilder
type TextTemplateBuilder struct{}

// Parse is a wrapper for template.Parse
func (TextTemplateBuilder) Parse(tmpl string) (TemplateApplier, error) {
	return template.New("mapping").Parse(tmpl)
}
