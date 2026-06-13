// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

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
