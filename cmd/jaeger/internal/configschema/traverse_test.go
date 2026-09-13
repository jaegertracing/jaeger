// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package configschema

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/configschema/testdata"
)

const localRefPrefix = "#/$defs/"

// buildEdgeCaseSchema builds a root JSON schema containing the definition of
// testdata.EdgeCaseConfig, using documentation loaded from the testdata directory.
func buildEdgeCaseSchema(t *testing.T) *Schema {
	t.Helper()

	g := newGenerator()
	testdataDir := filepath.Join(moduleRoot(), "cmd", "jaeger", "internal", "configschema", "testdata")
	require.NoError(t, g.docs.load(modulePath(), []string{testdataDir}))

	cfg := testdata.DefaultEdgeCaseConfig()
	root := &Schema{
		Schema:     "https://json-schema.org/draft/2020-12/schema",
		Type:       "object",
		Properties: map[string]*Schema{},
		Defs:       g.defs,
	}
	root.Properties["edge_case"] = g.typeToSchema(reflect.TypeOf(cfg), reflect.ValueOf(cfg))
	return root
}

// parseLocalRef extracts the $defs key from a local "#/$defs/<key>" reference.
func parseLocalRef(ref string) (string, bool) {
	key := strings.TrimPrefix(ref, localRefPrefix)
	return key, key != "" && key != ref
}

// requireProp returns the named property of a schema object or fails the test.
func requireProp(t *testing.T, s *Schema, name string) *Schema {
	t.Helper()
	prop, ok := s.Properties[name]
	require.Truef(t, ok, "property %q not found", name)
	return prop
}

func TestTraverse_EdgeCases(t *testing.T) {
	schema := buildEdgeCaseSchema(t)

	edgeCase := requireProp(t, schema, "edge_case")
	require.Equal(t, localRefPrefix+"testdata_EdgeCaseConfig", edgeCase.Ref)

	def, ok := schema.Defs["testdata_EdgeCaseConfig"]
	require.True(t, ok, "EdgeCaseConfig must have a definition in $defs")

	// The squashed embedded struct must be flattened into the parent schema.
	_, squashed := def.Properties["squashed"]
	require.False(t, squashed, "squashed embedded struct must not appear as a nested property")
	squashFlag := requireProp(t, def, "squash_flag")
	require.Equal(t, "boolean", squashFlag.Type)

	// map[string]int must become an object whose additionalProperties is integer.
	intMap := requireProp(t, def, "int_map")
	require.Equal(t, "object", intMap.Type)
	require.NotNil(t, intMap.AdditionalProperties, "map field must use additionalProperties")
	require.Equal(t, "integer", intMap.AdditionalProperties.Type)

	// []string must become an array of strings.
	stringSlice := requireProp(t, def, "string_slice")
	require.Equal(t, "array", stringSlice.Type)
	require.NotNil(t, stringSlice.Items, "slice field must declare items")
	require.Equal(t, "string", stringSlice.Items.Type)

	// A pointer to a named struct must be a $ref to a $defs entry.
	target := requireProp(t, def, "target")
	require.Equal(t, localRefPrefix+"testdata_EdgeTarget", target.Ref)
	_, hasTargetDef := schema.Defs["testdata_EdgeTarget"]
	require.True(t, hasTargetDef, "EdgeTarget must have a definition in $defs")

	// configoptional.Optional[int] must be a plain integer.
	maybeCount := requireProp(t, def, "maybe_count")
	require.Equal(t, "integer", maybeCount.Type)

	// A field documented as deprecated must be marked deprecated.
	deprecated := requireProp(t, def, "deprecated_field")
	require.True(t, deprecated.Deprecated)
}

func TestTraverse_NoDanglingRefs(t *testing.T) {
	schema := buildEdgeCaseSchema(t)

	var errs []string

	// Every $ref in the schema must resolve to an existing key in the root $defs.
	var refCheck func(s *Schema)
	refCheck = func(s *Schema) {
		if s == nil {
			return
		}
		if s.Ref != "" {
			key, ok := parseLocalRef(s.Ref)
			if !ok {
				errs = append(errs, fmt.Sprintf("ref %q is not a local %s reference", s.Ref, localRefPrefix))
			} else if _, exists := schema.Defs[key]; !exists {
				errs = append(errs, fmt.Sprintf("ref %q does not resolve to an existing key in $defs", s.Ref))
			}
		}
		for _, prop := range s.Properties {
			refCheck(prop)
		}
		for _, def := range s.Defs {
			refCheck(def)
		}
		refCheck(s.Items)
		refCheck(s.AdditionalProperties)
	}
	refCheck(schema)

	// A field whose Go type is a named struct must never be represented as a
	// plain "string" schema type.
	var checkGo func(v reflect.Value, s *Schema)
	checkGo = func(v reflect.Value, s *Schema) {
		if s == nil {
			return
		}
		if s.Ref != "" {
			if key, ok := parseLocalRef(s.Ref); ok {
				if def, exists := schema.Defs[key]; exists {
					s = def
				}
			}
		}
		if v.Kind() == reflect.Pointer {
			if v.IsNil() {
				return
			}
			v = v.Elem()
		}
		if v.Kind() != reflect.Struct {
			return
		}
		for i := range v.Type().NumField() {
			field := v.Type().Field(i)
			if !field.IsExported() {
				continue
			}
			tag := field.Tag.Get("mapstructure")
			if tag == "-" {
				continue
			}
			name, opts := parseTag(tag)
			if field.Anonymous && opts.Contains("squash") {
				// Flattened into the parent schema, so keep the same schema node.
				checkGo(v.Field(i), s)
				continue
			}
			if name == "" {
				name = lowerFirst(field.Name)
			}
			if name == "" {
				continue
			}
			prop := s.Properties[name]
			if prop == nil {
				continue
			}
			fieldType := field.Type
			for fieldType.Kind() == reflect.Pointer {
				fieldType = fieldType.Elem()
			}
			if fieldType.Kind() == reflect.Struct && fieldType.PkgPath() != "" && fieldType.Name() != "" && prop.Type == "string" {
				errs = append(errs, fmt.Sprintf("field %q of %s is a named struct but is represented as a plain \"string\" type", name, v.Type()))
			}
			checkGo(v.Field(i), prop)
		}
	}
	checkGo(reflect.ValueOf(testdata.DefaultEdgeCaseConfig()), schema.Properties["edge_case"])

	require.Empty(t, errs, "schema invariants violated:\n%s", strings.Join(errs, "\n"))
}
