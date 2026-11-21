// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package configschema

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

func renderMarkdown(schema *Schema, path string) error {
	var b strings.Builder
	b.WriteString("# Jaeger v2 Configuration Reference\n\n")
	if schema.Description != "" {
		b.WriteString(schema.Description)
		b.WriteString("\n\n")
	}

	rootKeys := sortedKeys(schema.Properties)
	for _, key := range rootKeys {
		comp := schema.Properties[key]
		b.WriteString(fmt.Sprintf("## %s\n\n", key))
		if comp.Description != "" {
			b.WriteString(comp.Description)
			b.WriteString("\n\n")
		}
		renderSchemaInto(&b, schema.Defs, key, comp, 3)
	}

	if len(schema.Defs) > 0 {
		b.WriteString("## Common configuration types\n\n")
		for _, name := range sortedKeys(schema.Defs) {
			def := schema.Defs[name]
			if usedOnlyByRoot(name, schema.Properties) {
				// Skip definitions that are already fully described as top-level components.
				continue
			}
			b.WriteString(fmt.Sprintf("### %s\n\n", name))
			renderSchemaInto(&b, schema.Defs, name, def, 4)
		}
	}

	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// usedOnlyByRoot returns true when the definition named name is referenced only
// from the root component list and therefore already documented there.
func usedOnlyByRoot(name string, props map[string]*Schema) bool {
	prefix := "#/$defs/" + name
	for _, p := range props {
		if p.Ref == prefix {
			return true
		}
	}
	return false
}

func renderSchemaInto(b *strings.Builder, defs map[string]*Schema, title string, s *Schema, baseLevel int) {
	if s.Ref != "" {
		name := strings.TrimPrefix(s.Ref, "#/$defs/")
		if d, ok := defs[name]; ok {
			s = d
		}
	}

	if s.Description != "" {
		b.WriteString(s.Description)
		b.WriteString("\n\n")
	}

	if s.Type == "array" && s.Items != nil {
		b.WriteString(fmt.Sprintf("- **Type:** array of %s\n", typeString(s.Items, defs)))
		if s.Description != "" {
			b.WriteString(fmt.Sprintf("- **Description:** %s\n", s.Description))
		}
		b.WriteString("\n")
		return
	}

	if len(s.Properties) == 0 {
		b.WriteString(fmt.Sprintf("- **Type:** %s\n", typeString(s, defs)))
		if s.Description != "" {
			b.WriteString(fmt.Sprintf("- **Description:** %s\n", s.Description))
		}
		b.WriteString("\n")
		return
	}

	required := make(map[string]struct{}, len(s.Required))
	for _, r := range s.Required {
		required[r] = struct{}{}
	}

	b.WriteString("| Key | Type | Required | Default | Description |\n")
	b.WriteString("| --- | ---- | -------- | ------- | ----------- |\n")
	for _, key := range sortedKeys(s.Properties) {
		prop := s.Properties[key]
		b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s |\n",
			escapeTableCell(key),
			escapeTableCell(typeString(prop, defs)),
			requiredMarker(key, required),
			escapeTableCell(defaultString(prop.Default)),
			escapeTableCell(prop.Description),
		))
	}
	b.WriteString("\n")
}

func requiredMarker(key string, required map[string]struct{}) string {
	if _, ok := required[key]; ok {
		return "yes"
	}
	return "no"
}

func typeString(s *Schema, defs map[string]*Schema) string {
	if s.Ref != "" {
		name := strings.TrimPrefix(s.Ref, "#/$defs/")
		return fmt.Sprintf("[%s](#%s)", name, name)
	}
	if s.Type == "array" && s.Items != nil {
		return "array of " + typeString(s.Items, defs)
	}
	if s.Type == "object" && s.AdditionalProperties != nil {
		return "map[string]" + typeString(s.AdditionalProperties, defs)
	}
	if s.Format != "" {
		return fmt.Sprintf("%s (%s)", s.Type, s.Format)
	}
	if s.Type != "" {
		return s.Type
	}
	return "object"
}

func defaultString(def any) string {
	if def == nil {
		return ""
	}
	switch v := def.(type) {
	case string:
		return fmt.Sprintf("%q", v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func escapeTableCell(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(s)
}

func sortedKeys(m map[string]*Schema) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
