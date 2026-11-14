// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package configschema

// Schema is a minimal JSON Schema 2020-12 representation used by the Jaeger
// v2 configuration schema generator.  Map fields are used for properties and
// definitions so that encoding/json emits their keys in sorted order, giving
// deterministic output.
type Schema struct {
	Schema               string             `json:"$schema,omitempty"`
	Ref                  string             `json:"$ref,omitempty"`
	Type                 string             `json:"type,omitempty"`
	Title                string             `json:"title,omitempty"`
	Description          string             `json:"description,omitempty"`
	Deprecated           bool               `json:"deprecated,omitempty"`
	Format               string             `json:"format,omitempty"`
	Default              any                `json:"default,omitempty"`
	Required             []string           `json:"required,omitempty"`
	Properties           map[string]*Schema `json:"properties,omitempty"`
	Defs                 map[string]*Schema `json:"$defs,omitempty"`
	Items                *Schema            `json:"items,omitempty"`
	AdditionalProperties *Schema            `json:"additionalProperties,omitempty"`
}

// newObjectSchema returns a Schema initialized as an empty object.
func newObjectSchema() *Schema {
	return &Schema{Type: "object", Properties: map[string]*Schema{}}
}

// newArraySchema returns a Schema initialized as an array with the given item schema.
func newArraySchema(items *Schema) *Schema {
	return &Schema{Type: "array", Items: items}
}
