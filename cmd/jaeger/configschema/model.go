// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package configschema

import "reflect"

// ConfigInfo represents metadata about a configuration struct
type ConfigInfo struct {
	Name        string      `json:"name"`         // e.g., "Config"
	PackagePath string      `json:"package_path"` // Full package import path
	Fields      []FieldInfo `json:"fields"`       // All struct fields
}

// FieldInfo represents metadata about a single struct field
type FieldInfo struct {
	Name       string       `json:"name"`              // Go field name
	JSONName   string       `json:"json_name"`         // JSON tag name
	Type       string       `json:"type"`              // Type as string
	Kind       reflect.Kind `json:"kind"`              // reflect.Kind
	Comment    string       `json:"comment"`           // Extracted from comments
	Default    any          `json:"default,omitempty"` // Current value
	Required   bool         `json:"required"`          // From tags
	Omitempty  bool         `json:"omitempty"`         // From json tag
	IsEmbedded bool         `json:"is_embedded"`       // Embedded struct
	IsPointer  bool         `json:"is_pointer"`        // Pointer type
}

// ConfigCollection holds all collected config information
type ConfigCollection struct {
	Configs []ConfigInfo `json:"configs"`
}
