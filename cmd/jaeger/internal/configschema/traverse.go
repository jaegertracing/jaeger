// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0
package configschema

import (
	"fmt"
	"go/ast"
	"go/token"
	"reflect"
	"strings"

	"golang.org/x/tools/go/packages"
)

// In traverse.go
func parseASTWithImports(file *ast.File, pkg *packages.Package) []StructDoc {
	// First parse current package's structs
	structs := parseAST(file, pkg.PkgPath)

	// Then recursively parse imported packages
	for _, imp := range pkg.Imports {
		for _, file := range imp.Syntax {
			// Prevent infinite loops with a visited map
			visited := make(map[string]bool)
			if !visited[imp.PkgPath] {
				visited[imp.PkgPath] = true
				structs = append(structs, parseASTWithImports(file, imp)...)
			}
		}
	}

	return structs
}

// parseAST traverses an AST to extract struct definitions.
func parseAST(file *ast.File, pkgPath string) []StructDoc {
	var structs []StructDoc

	ast.Inspect(file, func(n ast.Node) bool {
		genDecl, ok := n.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			return true
		}

		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}

			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				continue
			}

			structDoc := StructDoc{
				Name:        fmt.Sprintf("%s.%s", pkgPath, typeSpec.Name.Name),
				PackagePath: pkgPath,
				Description: extractComment(genDecl.Doc),
				Properties:  make(map[string]FieldDoc),
				Required:    []string{},
			}

			for _, field := range structType.Fields.List {
				// Deprecation detection
				isDeprecated := false
				if field.Doc != nil {
					for _, comment := range field.Doc.List {
						if strings.Contains(comment.Text, "Deprecated:") {
							isDeprecated = true
							break
						}
					}
				}

				fieldType := exprToString(field.Type, file)
				tag := ""
				if field.Tag != nil {
					tag = strings.Trim(field.Tag.Value, "`")
				}

				msTag := parseMapStructureTag(tag)
				fieldName := determineFieldName(field, fieldType, msTag)

				// Skip empty names from anonymous fields
				if fieldName == "" || !isValidJSONPropertyName(fieldName) {
					continue
				}

				fieldDoc := FieldDoc{
					Type:         fieldType,
					MapStructure: msTag,
					Description:  extractComment(field.Doc),
					Required:     isRequired(msTag),
					Deprecated:   isDeprecated,
				}

				if len(field.Names) > 0 {
					for _, name := range field.Names {
						structDoc.Properties[name.Name] = fieldDoc
					}
				} else {
					structDoc.Properties[fieldName] = fieldDoc
				}

				if fieldDoc.Required && !fieldDoc.Deprecated && fieldName != "" {
					structDoc.Required = append(structDoc.Required, fieldName)
				}
			}

			structs = append(structs, structDoc)
		}
		return true
	})

	return structs
}

func isValidJSONPropertyName(name string) bool {
	return name != "" && strings.TrimSpace(name) != "" &&
		!strings.ContainsAny(name, " \t\n\r") &&
		!strings.HasPrefix(name, "_") &&
		!strings.Contains(name, "$")
}

func exprToString(expr ast.Expr, file *ast.File) string {
	switch t := expr.(type) {
	case *ast.SelectorExpr:
		return t.Sel.Name
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + exprToString(t.X, file)
	case *ast.ArrayType:
		return "[]" + exprToString(t.Elt, file)
	default:
		return "unknown"
	}
}

func extractComment(group *ast.CommentGroup) string {
	if group == nil {
		return ""
	}
	return strings.TrimSpace(group.Text())
}

func parseMapStructureTag(tag string) string {
	st := reflect.StructTag(tag)
	return st.Get("mapstructure")
}

func isRequired(msTag string) bool {
	return !strings.Contains(msTag, "omitempty") && msTag != ""
}

func determineFieldName(field *ast.Field, fieldType, msTag string) string {
	// Handle mapstructure tag first
	if msTag != "" {
		parts := strings.SplitN(msTag, ",", 2)
		if parts[0] != "" {
			return parts[0]
		}
	}

	// Handle named fields
	if len(field.Names) > 0 {
		return field.Names[0].Name
	}

	// Handle embedded types
	return sanitizeFieldName(fieldType)
}

func sanitizeFieldName(name string) string {
	// Remove pointers and package qualifiers
	name = strings.TrimPrefix(name, "*")
	if idx := strings.LastIndex(name, "."); idx != -1 {
		name = name[idx+1:]
	}
	return name
}
