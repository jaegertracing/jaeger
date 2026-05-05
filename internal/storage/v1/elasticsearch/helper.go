// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"fmt"
	"net/http"
	"strings"

	escfg "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
)

func WriteMockMappingResponse(prefix escfg.IndexPrefix, w http.ResponseWriter, r *http.Request) bool {
	templateName := prefix.Apply("jaeger-span")
	validMappingResponse := []byte(fmt.Sprintf(`{
		%q: {
			"mappings": {
				"properties": {
					"scopeTag": { "type": "keyword" },
					"references": {
						"properties": {
							"tags": { "type": "nested" }
						}
					}
				}
			}
		}
	}`, templateName))
	if strings.Contains(r.URL.Path, "_template") {
		w.Write(validMappingResponse)
		return true
	}
	return false
}
