// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0
package configschema

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal"
)

func GenerateSchema(outputPath string) error {
	configs, err := collectConfigs()
	if err != nil {
		return err
	}
	pkgNames := collectPackages(configs)
	fmt.Println("Discovered packages:\n- " + strings.Join(pkgNames, "\n- "))

	pkgs, err := loadPackages(pkgNames)
	if err != nil {
		return err
	}

	schema, err := constructSchema(pkgs, configs)
	if err != nil {
		return err
	}

	// Add defaults
	addDefaults(schema, configs)

	return writeSchema(schema, outputPath)
}

// Collect configs from all registered OTEL components via factories.
func collectConfigs() ([]any, error) {
	factories, err := internal.Components()
	if err != nil {
		return nil, fmt.Errorf("failed to get components: %w", err)
	}
	var configs []any
	for _, f := range factories.Extensions {
		configs = append(configs, f.CreateDefaultConfig())
	}
	for _, f := range factories.Receivers {
		configs = append(configs, f.CreateDefaultConfig())
	}
	for _, f := range factories.Processors {
		configs = append(configs, f.CreateDefaultConfig())
	}
	for _, f := range factories.Exporters {
		configs = append(configs, f.CreateDefaultConfig())
	}
	return configs, nil
}

func collectPackages(configs []any) []string {
	packages := make(map[string]struct{})

	for _, cfg := range configs {
		t := reflect.TypeOf(cfg)
		if t.Kind() == reflect.Ptr {
			t = t.Elem()
		}

		// Add the config struct's own package
		if pkgPath := t.PkgPath(); pkgPath != "" {
			packages[pkgPath] = struct{}{}
		}

		// Add packages from all nested fields
		for i := range t.NumField() {
			field := t.Field(i)
			fieldType := field.Type

			// Dereference pointers/slices/maps
			for fieldType.Kind() == reflect.Ptr || fieldType.Kind() == reflect.Slice || fieldType.Kind() == reflect.Map {
				fieldType = fieldType.Elem()
			}

			if fieldType.Kind() == reflect.Struct {
				fieldInstance := reflect.New(fieldType).Interface()
				fieldPackages := collectPackages([]any{fieldInstance})
				for _, pkg := range fieldPackages {
					packages[pkg] = struct{}{}
				}
			}
		}
	}

	result := make([]string, 0, len(packages))
	for pkg := range packages {
		result = append(result, pkg)
	}
	slices.Sort(result)
	return result
}

func loadPackages(pkgPaths []string) ([]*packages.Package, error) {
	cfg := &packages.Config{
		Mode: packages.NeedSyntax | packages.NeedTypes | packages.NeedName,
	}
	pkgs, err := packages.Load(cfg, pkgPaths...)
	if err != nil {
		return nil, fmt.Errorf("failed to load packages: %w", err)
	}
	return pkgs, nil
}

func constructSchema(pkgs []*packages.Package, configs []any) (*JSONSchema, error) {
	schema := JSONSchema{
		Schema:      "https://json-schema.org/draft/2019-09/schema",
		Title:       "Jaeger Configuration",
		Description: "Jaeger component configuration schema",
		Type:        "object",
		Definitions: make(map[string]any),
		Properties:  make(map[string]any),
	}

	structRegistry := make(map[string]StructDoc)

	// First pass: Collect all structs
	for _, pkg := range pkgs {
		for _, syntax := range pkg.Syntax {
			for _, s := range parseASTWithImports(syntax, pkg) {
				structRegistry[s.Name] = s
			}
		}
	}

	// Second pass: Build schema with references
	for _, s := range structRegistry {
		schema.Definitions[s.Name] = buildSchemaStruct(s, structRegistry)
	}

	// Build root properties
	rootProps := make(map[string]any)
	// Inside GenerateDocs() function, after building rootProps:
	for key := range rootProps {
		if _, exists := schema.Definitions[getStructNameFromKey(key)]; !exists {
			return nil, fmt.Errorf("missing definition for component: %s", key)
		}
	}

	for _, cfg := range configs {
		key := getComponentKey(cfg)
		if key == "/" {
			continue // Skip root entry
		}
		t := reflect.TypeOf(cfg)
		if t.Kind() == reflect.Ptr {
			t = t.Elem()
		}
		pkgPath := t.PkgPath()
		structName := fmt.Sprintf("%s.%s", pkgPath, t.Name())

		// Skip invalid entries like "/"
		if pkgPath == "" || t.Name() == "" {
			continue
		}

		rootProps[getComponentKey(cfg)] = map[string]any{
			"$ref": fmt.Sprintf("#/definitions/%s", structName),
		}
	}
	schema.Properties = rootProps

	return &schema, nil
}

func buildSchemaStruct(s StructDoc, registry map[string]StructDoc) map[string]any {
	props := make(map[string]any)
	required := make([]string, 0)

	for name, field := range s.Properties {
		// skip invalid names processing
		if name == "" {
			continue
		}

		prop := map[string]any{
			"description": field.Description,
		}

		// Handle special types
		switch {
		case field.Type == "time.Duration":
			prop["type"] = "string"
			prop["format"] = "duration"
		case strings.HasPrefix(field.Type, "[]"):
			elementType := strings.TrimPrefix(field.Type, "[]")
			prop["type"] = "array"
			prop["items"] = map[string]any{
				"type": mapTypeToJSONSchema(elementType),
			}
		default:
			if nestedSchema, exists := registry[field.Type]; exists {
				// Existing $ref logic
				prop["$ref"] = fmt.Sprintf("#/definitions/%s", nestedSchema.Name)
			} else {
				// Handle external types not in registry
				if isCrossPackageType(field.Type, s.PackagePath) {
					prop["type"] = "object"
					prop["description"] = fmt.Sprintf("External type: %s", field.Type)
				} else {
					// Primitive type handling
					prop["type"] = mapTypeToJSONSchema(field.Type)
				}
			}
		}

		// Add deprecation handling
		if field.Deprecated {
			prop["deprecated"] = true
			prop["description"] = "[DEPRECATED] " + field.Description
		}

		if field.DefaultValue != nil {
			prop["default"] = field.DefaultValue
		}

		if nestedSchema, exists := registry[field.Type]; exists {
			prop["$ref"] = fmt.Sprintf("#/definitions/%s", nestedSchema.Name)
		} else {
			prop["type"] = mapTypeToJSONSchema(field.Type)
		}

		// Handle arrays
		if strings.HasPrefix(field.Type, "[]") {
			prop["type"] = "array"
			prop["items"] = map[string]any{
				"type": mapTypeToJSONSchema(field.Type[2:]),
			}
		}

		props[name] = prop

		// Only add to required if not deprecated
		if field.Required && !field.Deprecated {
			required = append(required, name)
		}
	}

	schema := map[string]any{
		"title":       s.Name,
		"type":        "object",
		"description": s.Description,
		"properties":  props,
	}

	if len(required) > 0 {
		schema["required"] = required
	}

	return schema
}

func getStructNameFromKey(key string) string {
	parts := strings.Split(key, "/")
	return parts[len(parts)-1]
}

func isCrossPackageType(typeName, currentPkgPath string) bool {
	// Check if type is qualified with a package
	if !strings.Contains(typeName, ".") {
		return false
	}

	// Extract package from type name (e.g., "configgrpc" from "configgrpc.GRPCServerSettings")
	parts := strings.Split(typeName, ".")
	typePkg := strings.Join(parts[:len(parts)-1], ".")

	// Compare with current package
	return typePkg != "" && typePkg != currentPkgPath
}

// Function to ADd defaults
func addDefaults(schema *JSONSchema, configs []any) {
	for _, cfg := range configs {
		structName := getStructName(cfg)
		defaults := getDefaults(cfg)

		if def, exists := schema.Definitions[structName].(map[string]any); exists {
			props := def["properties"].(map[string]any)
			for name, value := range defaults {
				if prop, ok := props[name].(map[string]any); ok {
					// Skip deprecated fields
					if _, deprecated := prop["deprecated"]; !deprecated {
						prop["default"] = value
					}
				}
			}
		}
	}
}

func mapTypeToJSONSchema(goType string) string {
	// Handle pointer types
	if strings.HasPrefix(goType, "*") {
		return mapTypeToJSONSchema(goType[1:])
	}

	// Handle complex types
	switch {
	// case goType == "time.Duration":
	// 	return "string"
	case strings.HasPrefix(goType, "[]"):
		return "array"
	// case strings.HasPrefix(goType, "map["):
	// 	return "object"
	case goType == "string":
		return "string"
	case goType == "bool":
		return "boolean"
	case strings.Contains(goType, "int"):
		return "integer"
	case strings.Contains(goType, "float"):
		return "number"
	default:
		// Check if it's a known struct
		// if strings.Contains(goType, ".") {
		// 	return "object"
		// }
		return "string"
	}
}

func getStructName(cfg any) string {
	t := reflect.TypeOf(cfg)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return fmt.Sprintf("%s.%s", t.PkgPath(), t.Name())
}

func getComponentKey(cfg any) string {
	t := reflect.TypeOf(cfg)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return fmt.Sprintf("%s/%s", t.PkgPath(), t.Name())
}

func getDefaults(cfg any) map[string]any {
	defaults := make(map[string]any)
	data, err := json.Marshal(cfg)
	if err != nil {
		return defaults
	}
	json.Unmarshal(data, &defaults)
	return defaults
}

func writeSchema(schema *JSONSchema, outputPath string) error {
	schema.Schema = "http://json-schema.org/draft-07/schema#"
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer file.Close()

	enc := json.NewEncoder(file)
	enc.SetIndent("", "  ")
	return enc.Encode(schema)
}
