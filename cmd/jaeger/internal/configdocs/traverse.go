// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0
package configdocs

import (
	"fmt"
	"go/ast"
	"go/token"
)

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
