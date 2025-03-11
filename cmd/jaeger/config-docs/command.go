// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package configdocs

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/token"
	"log"
	"os"
	"strings"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/collector/component"
	"golang.org/x/tools/go/packages"
)

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

// Command returns the config-docs command.
func Command() *cobra.Command {
	return &cobra.Command{
		Use:   "config-docs",
		Short: "Extracts and generates configuration documentation from structs",
		RunE: func(cmd *cobra.Command, args []string) error {
			return GenerateDocs()
		},
	}
}

func GenerateDocs() error {
	// List of target directories for AST parsing
	targetPackages := []string{
		"github.com/jaegertracing/jaeger/cmd/jaeger/internal/exporters/storageexporter",
		"github.com/jaegertracing/jaeger/pkg/es/config",
		"go.opentelemetry.io/otel",
	}

	schema := map[string]interface{}{
		"$schema":     "https://json-schema.org/draft/2020-12/schema",
		"type":        "object",
		"definitions": map[string]interface{}{},
	}

	// Load packages using `go/packages`
	cfg := &packages.Config{
		Mode: packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedFiles,
	}
	pkgs, err := packages.Load(cfg, targetPackages...)
	if err != nil {
		log.Fatalf("Error loading packages: %v", err)
	}

	// Extract struct definitions from AST
	for _, pkg := range pkgs {
		for _, syntax := range pkg.Syntax {
			structs := parseAST(syntax)
			for _, structDoc := range structs {
				schema["definitions"].(map[string]interface{})[structDoc.Name] = structToSchema(structDoc)
			}
		}
	}

	// Extract referenced struct names from `components.go`
	referencedStructs := extractStructsFromComponents()

	// Extract defaults and embed them within the JSON Schema
	for structName := range referencedStructs {
		defaults := extractDefaults(structName)
		if schemaDef, exists := schema["definitions"].(map[string]interface{})[structName]; exists {
			// Embed default values into the JSON Schema definition
			if schemaMap, ok := schemaDef.(map[string]interface{}); ok {
				for fieldName, defaultValue := range defaults {
					if properties, ok := schemaMap["properties"].(map[string]interface{}); ok {
						if fieldSchema, ok := properties[fieldName].(map[string]interface{}); ok {
							fieldSchema["default"] = defaultValue
						}
					}
				}
			}
		}
	}

	// Serialize the collected struct documentation to JSON
	outputFile, err := os.Create("struct_docs.json")
	if err != nil {
		log.Fatalf("Error creating output file: %v", err)
	}
	defer outputFile.Close()

	encoder := json.NewEncoder(outputFile)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(schema); err != nil {
		log.Fatalf("Error encoding JSON: %v", err)
	}

	fmt.Println("Struct documentation has been written to struct_docs.json")
	return nil
}

// parseAST traverses an AST to extract struct definitions.
func parseAST(node ast.Node) []StructDoc {
	var structs []StructDoc

	ast.Inspect(node, func(n ast.Node) bool {
		switch t := n.(type) {
		case *ast.GenDecl:
			if t.Tok == token.TYPE {
				for _, spec := range t.Specs {
					typeSpec, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					structType, ok := typeSpec.Type.(*ast.StructType)
					if !ok {
						continue
					}

					structDoc := StructDoc{Name: typeSpec.Name.Name}
					if t.Doc != nil {
						structDoc.Comment = t.Doc.Text()
					}

					for _, field := range structType.Fields.List {
						fieldType := fmt.Sprint(field.Type)
						var tag string
						if field.Tag != nil {
							tag = field.Tag.Value
						}

						for _, fieldName := range field.Names {
							structDoc.Fields = append(structDoc.Fields, FieldDoc{
								Name: fieldName.Name,
								Type: fieldType,
								Tag:  tag,
							})
						}
					}
					structs = append(structs, structDoc)
				}
			}
		}
		return true
	})

	return structs
}

// extractStructsFromComponents extracts struct names referenced in `components.go`.
func extractStructsFromComponents() map[string]bool {
	structNames := make(map[string]bool)

	// Load components.go (adjust path if needed)
	cfg := &packages.Config{
		Mode: packages.NeedSyntax | packages.NeedFiles,
	}
	pkgs, err := packages.Load(cfg, "github.com/jaegertracing/jaeger/cmd/jaeger/internal/components")
	if err != nil {
		log.Fatalf("Error loading components.go: %v", err)
	}

	// Parse AST to find struct names used in components.go
	for _, pkg := range pkgs {
		for _, syntax := range pkg.Syntax {
			ast.Inspect(syntax, func(n ast.Node) bool {
				if expr, ok := n.(*ast.CompositeLit); ok {
					if ident, ok := expr.Type.(*ast.Ident); ok {
						structNames[ident.Name] = true
					}
				}
				return true
			})
		}
	}
	return structNames
}

func structToSchema(structDoc StructDoc) map[string]interface{} {
	properties := make(map[string]interface{})
	requiredFields := []string{}

	for _, field := range structDoc.Fields {
		fieldSchema := map[string]interface{}{
			"type": mapGoTypeToJSONType(field.Type),
		}

		if field.Comment != "" {
			fieldSchema["description"] = field.Comment
		}
		if field.DefaultValue != nil {
			fieldSchema["default"] = field.DefaultValue
		}

		properties[field.Name] = fieldSchema
		requiredFields = append(requiredFields, field.Name)
	}

	return map[string]interface{}{
		"type":        "object",
		"properties":  properties,
		"required":    requiredFields,
		"description": structDoc.Comment,
	}
}

func mapGoTypeToJSONType(goType string) string {
	switch goType {
	case "int", "int32", "int64", "uint", "uint32", "uint64":
		return "integer"
	case "float32", "float64":
		return "number"
	case "bool":
		return "boolean"
	case "string":
		return "string"
	case "[]string", "[]int", "[]float64", "[]bool":
		return "array"
	case "map[string]string", "map[string]int":
		return "object"
	default:
		return "string" // Fallback for complex types
	}
}

// extractDefaults extracts default values only for relevant structs.
func extractDefaults(structName string) map[string]interface{} {
	defaults := make(map[string]interface{})

	// Call Jaeger components initializer
	factories, err := internal.Components()
	if err != nil {
		log.Fatalf("Error building factories: %v", err)
	}

	// Iterate over all factory types and fetch defaults only if the struct matches
	for _, factory := range factories.Extensions {
		if matchAndProcessFactory(factory, structName, defaults) {
			return defaults
		}
	}
	for _, factory := range factories.Receivers {
		if matchAndProcessFactory(factory, structName, defaults) {
			return defaults
		}
	}
	for _, factory := range factories.Processors {
		if matchAndProcessFactory(factory, structName, defaults) {
			return defaults
		}
	}
	for _, factory := range factories.Exporters {
		if matchAndProcessFactory(factory, structName, defaults) {
			return defaults
		}
	}

	return defaults
}

// matchAndProcessFactory checks if the struct matches and extracts default values.
func matchAndProcessFactory(factory component.Factory, structName string, defaults map[string]interface{}) bool {
	cfg := factory.CreateDefaultConfig()
	configStructName := fmt.Sprintf("%T", cfg)
	configStructName = strings.TrimPrefix(configStructName, "*") // Remove pointer notation

	if configStructName == structName {
		jsonData, err := json.Marshal(cfg)
		if err == nil {
			json.Unmarshal(jsonData, &defaults)
		}
		return true
	}
	return false
}
