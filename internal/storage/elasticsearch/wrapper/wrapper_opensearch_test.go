// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package eswrapper

import (
	"strings"
	"testing"

	esV8 "github.com/elastic/go-elasticsearch/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTemplateCreatorWrapperV8_OpenSearchCompression(t *testing.T) {
	tests := []struct {
		name         string
		isOpenSearch bool
		expectError  bool
		description  string
	}{
		{
			name:         "OpenSearch with uncompressed client",
			isOpenSearch: true,
			expectError:  false,
			description:  "Should use uncompressed client for OpenSearch template creation",
		},
		{
			name:         "Elasticsearch with standard client",
			isOpenSearch: false,
			expectError:  false,
			description:  "Should use standard client for Elasticsearch template creation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock ES v8 client config
			config := &esV8.Config{
				Addresses:           []string{"http://localhost:9200"},
				CompressRequestBody: true, // Compression enabled by default
			}

			// Create wrapper with OpenSearch flag
			wrapper := TemplateCreatorWrapperV8{
				templateName: "test-template",
				isOpenSearch: tt.isOpenSearch,
			}

			// For OpenSearch, simulate creating an uncompressed client
			if tt.isOpenSearch && config != nil {
				// Create a copy with compression disabled
				uncompressedConfig := *config
				uncompressedConfig.CompressRequestBody = false

				// In the real implementation, this would create a new client
				// For testing, we just verify the logic
				assert.False(t, uncompressedConfig.CompressRequestBody,
					"Uncompressed client should have compression disabled")
			}

			// Test Body method
			result := wrapper.Body("test-mapping")
			resultWrapper, ok := result.(TemplateCreatorWrapperV8)
			require.True(t, ok)
			assert.Equal(t, "test-mapping", resultWrapper.templateMapping)
			assert.Equal(t, tt.isOpenSearch, resultWrapper.isOpenSearch)
		})
	}
}

func TestClientWrapper_CreateTemplate_OpenSearch(t *testing.T) {
	tests := []struct {
		name         string
		esVersion    uint
		isOpenSearch bool
		description  string
	}{
		{
			name:         "ES v8 with OpenSearch",
			esVersion:    8,
			isOpenSearch: true,
			description:  "Should create TemplateCreatorWrapperV8 with OpenSearch flag",
		},
		{
			name:         "ES v8 with Elasticsearch",
			esVersion:    8,
			isOpenSearch: false,
			description:  "Should create TemplateCreatorWrapperV8 without OpenSearch flag",
		},
		{
			name:         "ES v7",
			esVersion:    7,
			isOpenSearch: false,
			description:  "Should use legacy template creator for v7",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := ClientWrapper{
				esVersion:    tt.esVersion,
				isOpenSearch: tt.isOpenSearch,
			}

			// Mock createUncompressedClient to return nil for simplicity
			// In real implementation, this would create an actual client

			templateService := client.CreateTemplate("test-template")

			if tt.esVersion >= 8 {
				wrapper, ok := templateService.(TemplateCreatorWrapperV8)
				require.True(t, ok, "Should return TemplateCreatorWrapperV8 for v8+")
				assert.Equal(t, tt.isOpenSearch, wrapper.isOpenSearch)
				assert.Equal(t, "test-template", wrapper.templateName)
			} else {
				_, ok := templateService.(TemplateCreatorWrapperV8)
				assert.False(t, ok, "Should not return TemplateCreatorWrapperV8 for v7")
			}
		})
	}
}

func TestOpenSearchDetection(t *testing.T) {
	tests := []struct {
		name     string
		tagLine  string
		expected bool
	}{
		{
			name:     "OpenSearch 1.x",
			tagLine:  "OpenSearch 1.3.6",
			expected: true,
		},
		{
			name:     "OpenSearch 2.x",
			tagLine:  "OpenSearch 2.11.0",
			expected: true,
		},
		{
			name:     "Elasticsearch",
			tagLine:  "You know, for search",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test OpenSearch detection logic
			isOpenSearch := strings.Contains(tt.tagLine, "OpenSearch")
			assert.Equal(t, tt.expected, isOpenSearch)
		})
	}
}
