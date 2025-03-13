// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0
package configdocs

// FieldDoc represents documentation for a struct field.
type FieldDoc struct {
	Name           string      `json:"-"`
	Type           string      `json:"type"`
	MapStructure   string      `json:"mapstructure,omitempty"`
	Description    string      `json:"description,omitempty"`
	DefaultValue   interface{} `json:"default,omitempty"`
	Required       bool        `json:"required,omitempty"`
	JSONSchemaType string      `json:"-"`
	NestedType     string      `json:"$ref,omitempty"`
	Deprecated     bool        `json:"deprecated,omitempty"`
}

// StructDoc represents documentation for a struct.
type StructDoc struct {
	Name        string              `json:"-"`
	PackagePath string              `json:"-"`
	Description string              `json:"description,omitempty"`
	Properties  map[string]FieldDoc `json:"properties"`
	Required    []string            `json:"required,omitempty"`
}
