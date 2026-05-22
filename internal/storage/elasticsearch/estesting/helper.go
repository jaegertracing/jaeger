// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package estesting

import (
	"fmt"
	"net/http"
	"strings"
)

func WriteMockMappingResponse(w http.ResponseWriter, r *http.Request) bool {
	templateName := "jaeger-span"
	if strings.Contains(r.URL.Path, "_index_template") {
		validIndexTemplateResponse := []byte(fmt.Sprintf(`{
			"index_templates": [
				{
					"name": %q,
					"index_template": {
						"template": {
							"mappings": {
								"properties": {
									"scopeTag": { "type": "keyword" },
									"references": {
										"properties": {
											"tags": { "type": "nested" },
											"traceState": { "type": "keyword" },
											"flags": { "type": "integer" }
										}
									}
								}
							}
						}
					}
				}
			]
		}`, templateName))
		w.Write(validIndexTemplateResponse)
		return true
	}
	if strings.Contains(r.URL.Path, "_template") {
		validMappingResponse := []byte(fmt.Sprintf(`{
			%q: {
				"mappings": {
					"properties": {
						"scopeTag": { "type": "keyword" },
						"references": {
							"properties": {
								"tags": { "type": "nested" },
								"traceState": { "type": "keyword" },
								"flags": { "type": "integer" }
							}
						}
					}
				}
			}
		}`, templateName))
		w.Write(validMappingResponse)
		return true
	}
	return false
}
