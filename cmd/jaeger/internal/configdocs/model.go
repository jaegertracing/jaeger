// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0
package configdocs

// FieldDoc represents documentation for a struct field.
type FieldDoc struct {
	Name         string      `json:"name"`
	Type         string      `json:"type"`
	Tag          string      `json:"tag,omitempty"`
	DefaultValue interface{} `json:"default_value,omitempty"`
	Comment      string      `json:"comment,omitempty"`
}

// StructDoc represents documentation for a struct.
type StructDoc struct {
	Name    string     `json:"name"`
	Fields  []FieldDoc `json:"fields"`
	Comment string     `json:"comment,omitempty"`
}
