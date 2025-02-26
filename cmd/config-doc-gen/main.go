// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

func main() {
	// List of target directories
	targetDirs := []string{
		"cmd/jaeger/internal/extension/jaegerquery",
		"pkg/es/config",
	}

	var allStructs []StructDoc

	// Iterate over each target directory
	for _, dir := range targetDirs {
		fmt.Printf("Parsing directory: %s\n", dir)
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			// Process only Go source files
			if !info.IsDir() && filepath.Ext(path) == ".go" {
				structs, err := parseFile(path)
				if err != nil {
					log.Printf("Error parsing file %s: %v", path, err)
					return err
				}
				allStructs = append(allStructs, structs...)
			}
			return nil
		})
		if err != nil {
			log.Fatalf("Error walking the path %s: %v", dir, err)
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
	if err := encoder.Encode(allStructs); err != nil {
		log.Fatalf("Error encoding JSON: %v", err)
	}

	fmt.Println("Struct documentation has been written to struct_docs.json")
}

// parseFile parses a Go source file and extracts struct information.
func parseFile(filePath string) ([]StructDoc, error) {
	var structs []StructDoc

	// Create a new token file set
	fset := token.NewFileSet()

	// Parse the Go source file
	node, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse file %s: %w", filePath, err)
	}

	// Traverse the AST to find struct type declarations
	ast.Inspect(node, func(n ast.Node) bool {
		// Check for type declarations
		genDecl, ok := n.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			return true
		}

		// Iterate over the type specifications
		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}

			// Check if the type is a struct
			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				continue
			}

			structDoc := StructDoc{
				Name:    typeSpec.Name.Name,
				Comment: extractComment(genDecl.Doc),
			}

			// Iterate over the struct fields
			for _, field := range structType.Fields.List {
				fieldType := exprToString(fset, field.Type)
				fieldTag := extractTag(field.Tag)
				defaultValue := extractDefaultValue(field.Tag, fieldType)

				for _, name := range field.Names {
					structDoc.Fields = append(structDoc.Fields, FieldDoc{
						Name:         name.Name,
						Type:         fieldType,
						Tag:          fieldTag,
						DefaultValue: defaultValue,
						Comment:      extractComment(field.Doc),
					})
				}
			}
			structs = append(structs, structDoc)
		}
		return false
	})

	return structs, nil
}

// extractComment retrieves the text from a CommentGroup.
func extractComment(cg *ast.CommentGroup) string {
	if cg != nil {
		return strings.TrimSpace(cg.Text())
	}
	return ""
}

// exprToString converts an ast.Expr to its string representation.
func exprToString(fset *token.FileSet, expr ast.Expr) string {
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, expr); err != nil {
		return ""
	}
	return buf.String()
}

// extractTag extracts the tag value from a BasicLit.
func extractTag(tag *ast.BasicLit) string {
	if tag != nil {
		return strings.Trim(tag.Value, "`")
	}
	return ""
}

// extractDefaultValue parses the tag to find a default value if specified.
func extractDefaultValue(tag *ast.BasicLit, fieldType string) interface{} {
	if tag == nil {
		return nil
	}
	tagValue := extractTag(tag)
	structTag := reflect.StructTag(tagValue)
	defaultValueStr := structTag.Get("default")
	if defaultValueStr == "" {
		return nil
	}

	// Convert the default value string to the appropriate type
	switch fieldType {
	case "string":
		return defaultValueStr
	case "int", "int8", "int16", "int32", "int64":
		var intValue int64
		fmt.Sscanf(defaultValueStr, "%d", &intValue)
		return intValue
	case "uint", "uint8", "uint16", "uint32", "uint64":
		var uintValue uint64
		fmt.Sscanf(defaultValueStr, "%d", &uintValue)
		return uintValue
	case "float32", "float64":
		var floatValue float64
		fmt.Sscanf(defaultValueStr, "%f", &floatValue)
		return floatValue
	case "bool":
		var boolValue bool
		fmt.Sscanf(defaultValueStr, "%t", &boolValue)
		return boolValue
	// Add more types as needed
	default:
		return nil
	}
}
