// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package apiv3

import (
	"embed"
	"strings"
	"testing"
	"unicode"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"
)

//go:embed testdata/query_service.openapi.yaml
var openAPISpec embed.FS

func TestOpenAPIConformance_queryParamsMatchRuntime(t *testing.T) {
	spec := loadOpenAPISpec(t)
	for path, pathItem := range spec.Paths.Map() {
		for _, op := range pathItem.Operations() {
			if op == nil {
				continue
			}
			for _, param := range op.Parameters {
				if param.Value == nil || param.Value.In != "query" {
					continue
				}
				name := param.Value.Name
				if _, ok := canonicalQueryParams[name]; !ok {
					t.Fatalf("OpenAPI query param %q on path %s is not recognized by HTTP gateway canonical params", name, path)
				}
			}
		}
	}
}

func TestOpenAPIConformance_responseFieldsUseCamelCase(t *testing.T) {
	spec := loadOpenAPISpec(t)
	for name, schemaRef := range spec.Components.Schemas {
		if schemaRef.Value == nil {
			continue
		}
		assertCamelCaseProperties(t, name, schemaRef.Value.Properties)
	}
}

func loadOpenAPISpec(t *testing.T) *openapi3.T {
	t.Helper()
	data, err := openAPISpec.ReadFile("testdata/query_service.openapi.yaml")
	require.NoError(t, err)
	loader := openapi3.NewLoader()
	spec, err := loader.LoadFromData(data)
	require.NoError(t, err)
	require.NoError(t, spec.Validate(loader.Context))
	return spec
}

func assertCamelCaseProperties(t *testing.T, schemaName string, props openapi3.Schemas) {
	t.Helper()
	for propName, propRef := range props {
		if propName == "@type" {
			continue
		}
		if strings.Contains(propName, "_") {
			t.Fatalf("schema %s property %q uses snake_case; expected proto3 JSON camelCase", schemaName, propName)
		}
		if propName != "" && !unicode.IsLower(rune(propName[0])) && propName != "ID" {
			t.Fatalf("schema %s property %q must use lowerCamelCase", schemaName, propName)
		}
		if propRef.Value != nil && len(propRef.Value.Properties) > 0 {
			assertCamelCaseProperties(t, schemaName+"."+propName, propRef.Value.Properties)
		}
	}
}
