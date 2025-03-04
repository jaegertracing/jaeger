// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	"log"
	"os"
	"strings"

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

func main() {
	// List of target directories
	targetPackages := []string{
		"github.com/jaegertracing/jaeger/cmd/jaeger/internal/exporters/storageexporter",
		"github.com/jaegertracing/jaeger/pkg/es/config",
		"go.opentelemetry.io/otel",
	}

	var allStructs []StructDoc

	// Load packages using `go/packages`
	cfg := &packages.Config{
		Mode: packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedFiles | packages.LoadImports,
	}
	pkgs, err := packages.Load(cfg, targetPackages...)

	if err != nil {
		log.Fatalf("Error loading packages: %v", err)
	}

	// Process each package
	for _, pkg := range pkgs {
		for _, syntax := range pkg.Syntax { // syntax contains AST trees of all files
			structs := parseAST(syntax)
			allStructs = append(allStructs, structs...)
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

// processField processes a struct field and handles nested structs.
func processField(fset *token.FileSet, field *ast.Field, structs map[string]StructDoc) []FieldDoc {
	var fieldDocs []FieldDoc
	fieldType := exprToString(fset, field.Type)
	fieldTag := extractTag(field.Tag)

	for _, name := range field.Names {
		fieldDoc := FieldDoc{
			Name:    name.Name,
			Type:    fieldType,
			Tag:     fieldTag,
			Comment: extractComment(field.Doc),
		}
		fieldDocs = append(fieldDocs, fieldDoc)
	}

	// Handle nested struct
	if ident, ok := field.Type.(*ast.Ident); ok && ident.Obj != nil {
		if typeSpec, ok := ident.Obj.Decl.(*ast.TypeSpec); ok {
			if structType, isStruct := typeSpec.Type.(*ast.StructType); isStruct {
				nestedStructDoc := StructDoc{
					Name:    ident.Name,
					Comment: extractComment(typeSpec.Doc),
				}
				for _, nestedField := range structType.Fields.List {
					nestedStructDoc.Fields = append(nestedStructDoc.Fields, processField(fset, nestedField, structs)...)
				}
				structs[ident.Name] = nestedStructDoc
			}
		}
	}

	return fieldDocs
}

// parseFile parses a Go source file and extracts struct information.
func parseAST(node *ast.File) []StructDoc {
	var structs []StructDoc
	structMap := make(map[string]StructDoc)
	defaultValues := make(map[string]map[string]interface{}) // Store default values for structs

	// Create a new token file set
	fset := token.NewFileSet()

	// Traverse the AST to find struct type declarations and functions
	ast.Inspect(node, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.GenDecl: // Handle struct declarations
			if n.Tok != token.TYPE {
				return true
			}
			for _, spec := range n.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				structType, ok := typeSpec.Type.(*ast.StructType)
				if !ok {
					continue
				}

				structDoc := StructDoc{
					Name:    typeSpec.Name.Name,
					Comment: extractComment(n.Doc),
				}

				for _, field := range structType.Fields.List {
					structDoc.Fields = append(structDoc.Fields, processField(fset, field, structMap)...)
				}

				structMap[structDoc.Name] = structDoc
			}

		case *ast.FuncDecl: // Handle `createDefaultConfig()`
			if n.Name.Name == "createDefaultConfig" {
				structName, defaults := extractDefaultValues(fset, n)

				if structName != "" {
					defaultValues[structName] = defaults
				}
			}
		}
		return true
	})

	// Apply extracted defaults to structs
	for structName, defaults := range defaultValues {
		if structDoc, exists := structMap[structName]; exists {
			for i, field := range structDoc.Fields {
				if val, found := defaults[field.Name]; found {
					structDoc.Fields[i].DefaultValue = val
				}
			}
			structMap[structName] = structDoc
		}
	}

	// Convert map to slice
	for _, structDoc := range structMap {
		structs = append(structs, structDoc)
	}
	return structs
}

// function to extract default value from struct
func extractDefaultValues(fset *token.FileSet, fn *ast.FuncDecl) (string, map[string]interface{}) {
	defaults := make(map[string]interface{})
	var structName string

	if fn.Body.List == nil {
		return "", defaults
	}

	for _, stmt := range fn.Body.List {
		retStmt, ok := stmt.(*ast.ReturnStmt)
		if !ok || len(retStmt.Results) == 0 {
			continue
		}

		unaryExpr, ok := retStmt.Results[0].(*ast.UnaryExpr)
		if !ok {
			continue
		}

		compLit, ok := unaryExpr.X.(*ast.CompositeLit)
		if !ok {
			continue
		}

		if ident, ok := compLit.Type.(*ast.Ident); ok {
			structName = ident.Name
		}

		for _, elt := range compLit.Elts {
			kvExpr, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}

			// Pass a valid fset instead of nil
			fieldName := exprToString(fset, kvExpr.Key)
			defaultValue := extractValue(kvExpr.Value)
			defaults[fieldName] = defaultValue
		}
	}

	return structName, defaults
}

// function to extract value
func extractValue(expr ast.Expr) interface{} {
	switch v := expr.(type) {
	case *ast.BasicLit: // Handle basic types
		switch v.Kind {
		case token.STRING:
			return strings.Trim(v.Value, `"`)
		case token.INT:
			var intValue int
			fmt.Sscanf(v.Value, "%d", &intValue)
			return intValue
		case token.FLOAT:
			var floatValue float64
			fmt.Sscanf(v.Value, "%f", &floatValue)
			return floatValue
		default:
			return v.Value
		}
	case *ast.Ident: // Handle boolean values
		if v.Name == "true" {
			return true
		} else if v.Name == "false" {
			return false
		}
		return v.Name // Could be a constant
	case *ast.CallExpr: // Handle function calls
		if fun, ok := v.Fun.(*ast.SelectorExpr); ok {
			return fmt.Sprintf("function_call: %s.%s", fun.X, fun.Sel.Name)
		} else if fun, ok := v.Fun.(*ast.Ident); ok {
			return fmt.Sprintf("function_call: %s", fun.Name)
		}
	}
	return nil
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
