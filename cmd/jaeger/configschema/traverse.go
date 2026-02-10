// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package configschema

import (
	"fmt"
	"go/ast"
	"reflect"
	"strings"

	"golang.org/x/tools/go/packages"
)

// extractConfigInfo extracts metadata from config objects
func extractConfigInfo(configs []any) (*ConfigCollection, error) {
	collection := &ConfigCollection{
		Configs: make([]ConfigInfo, 0, len(configs)),
	}

	for _, cfg := range configs {
		info, err := traverseConfig(cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to traverse config: %w", err)
		}
		collection.Configs = append(collection.Configs, *info)
	}

	return collection, nil
}

// traverseConfig extracts information from a single config object
func traverseConfig(cfg any) (*ConfigInfo, error) {
	v := reflect.ValueOf(cfg)
	t := reflect.TypeOf(cfg)

	// Dereference pointer
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
		v = v.Elem()
	}

	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("expected struct, got %v", t.Kind())
	}

	info := &ConfigInfo{
		Name:        t.Name(),
		PackagePath: t.PkgPath(),
		Fields:      make([]FieldInfo, 0),
	}

	// Traverse all fields
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fieldValue := v.Field(i)

		fieldInfo := extractFieldInfo(field, fieldValue)
		info.Fields = append(info.Fields, fieldInfo)
	}

	return info, nil
}

// extractFieldInfo extracts information from a single struct field
func extractFieldInfo(field reflect.StructField, value reflect.Value) FieldInfo {
	info := FieldInfo{
		Name:       field.Name,
		Type:       field.Type.String(),
		Kind:       field.Type.Kind(),
		IsEmbedded: field.Anonymous,
		IsPointer:  field.Type.Kind() == reflect.Ptr,
	}

	// Extract JSON tag
	if jsonTag, ok := field.Tag.Lookup("json"); ok {
		info.JSONName, info.Omitempty = parseJSONTag(jsonTag)
	} else {
		info.JSONName = field.Name
	}

	// Skip fields with json:"-"
	if info.JSONName == "-" {
		info.JSONName = ""
	}

	// Extract required from tags
	info.Required = isFieldRequired(field.Tag)

	// Get default value (if accessible)
	if value.IsValid() && value.CanInterface() {
		if !value.IsZero() {
			info.Default = value.Interface()
		}
	}

	return info
}

// parseJSONTag parses the json struct tag
func parseJSONTag(tag string) (name string, omitempty bool) {
	parts := strings.Split(tag, ",")
	if len(parts) == 0 {
		return "", false
	}

	name = parts[0]
	if name == "-" {
		return "-", false
	}

	// Check for omitempty
	for _, part := range parts[1:] {
		if part == "omitempty" {
			omitempty = true
			break
		}
	}

	return name, omitempty
}

// isFieldRequired checks if a field is required based on struct tags
func isFieldRequired(tag reflect.StructTag) bool {
	// Check mapstructure tag for ",required"
	if mapTag, ok := tag.Lookup("mapstructure"); ok {
		if strings.Contains(mapTag, ",required") {
			return true
		}
	}

	// Check valid tag for "required"
	if validTag, ok := tag.Lookup("valid"); ok {
		if strings.Contains(validTag, "required") {
			return true
		}
	}

	return false
}

// ASTCache holds parsed package information
type ASTCache struct {
	typeSpecs map[string]*ast.TypeSpec
}

// buildASTCache loads packages and builds cache of type specs
func buildASTCache(configs []any) (*ASTCache, error) {
	// Collect unique package paths
	pkgPaths := make(map[string]bool)
	for _, cfg := range configs {
		t := reflect.TypeOf(cfg)
		if t.Kind() == reflect.Ptr {
			t = t.Elem()
		}
		if t.PkgPath() != "" {
			pkgPaths[t.PkgPath()] = true
		}
	}

	// Convert to slice
	paths := make([]string, 0, len(pkgPaths))
	for path := range pkgPaths {
		paths = append(paths, path)
	}

	// Load packages with AST
	cfg := &packages.Config{
		Mode: packages.NeedName |
			packages.NeedFiles |
			packages.NeedSyntax |
			packages.NeedTypes,
	}

	pkgs, err := packages.Load(cfg, paths...)
	if err != nil {
		return nil, fmt.Errorf("failed to load packages: %w", err)
	}

	// Build cache
	cache := &ASTCache{
		typeSpecs: make(map[string]*ast.TypeSpec),
	}

	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			ast.Inspect(file, func(n ast.Node) bool {
				if typeSpec, ok := n.(*ast.TypeSpec); ok {
					fullName := pkg.PkgPath + "." + typeSpec.Name.Name
					cache.typeSpecs[fullName] = typeSpec
				}
				return true
			})
		}
	}

	return cache, nil
}

// extractComments extracts comments for all fields
func extractComments(info *ConfigInfo, cache *ASTCache) error {
	fullName := info.PackagePath + "." + info.Name
	typeSpec, ok := cache.typeSpecs[fullName]
	if !ok {
		return nil // Type not found, skip
	}

	structType, ok := typeSpec.Type.(*ast.StructType)
	if !ok {
		return nil
	}

	// Map field names for lookup
	fieldMap := make(map[string]*FieldInfo)
	for i := range info.Fields {
		fieldMap[info.Fields[i].Name] = &info.Fields[i]
	}

	// Extract comments
	for _, field := range structType.Fields.List {
		for _, name := range field.Names {
			if fieldInfo, ok := fieldMap[name.Name]; ok {
				comment := extractFieldComment(field)
				fieldInfo.Comment = comment
			}
		}
	}

	return nil
}

// extractFieldComment extracts comment from AST field
func extractFieldComment(field *ast.Field) string {
	var comment string

	if field.Doc != nil {
		comment = field.Doc.Text()
	}

	if comment == "" && field.Comment != nil {
		comment = field.Comment.Text()
	}

	comment = strings.TrimSpace(comment)
	comment = strings.TrimPrefix(comment, "//")
	comment = strings.TrimSpace(comment)

	return comment
}

// extractConfigInfoWithComments extracts info with comments
func extractConfigInfoWithComments(configs []any) (*ConfigCollection, error) {
	// Build AST cache
	cache, err := buildASTCache(configs)
	if err != nil {
		return nil, fmt.Errorf("failed to build AST cache: %w", err)
	}

	// Extract config info
	collection, err := extractConfigInfo(configs)
	if err != nil {
		return nil, err
	}

	// Add comments
	for i := range collection.Configs {
		if err := extractComments(&collection.Configs[i], cache); err != nil {
			return nil, fmt.Errorf("failed to extract comments: %w", err)
		}
	}

	return collection, nil
}
