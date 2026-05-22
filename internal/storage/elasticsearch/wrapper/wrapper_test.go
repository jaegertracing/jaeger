// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package eswrapper

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/olivere/elastic/v7"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/estesting"
)

var mockEsServerResponse = []byte(`
{
	"Version": {
		"Number": "6"
	}
}
`)

func TestClientWrapper_GetTemplateMappings(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if estesting.WriteMockMappingResponse(w, r) {
			return
		}
		w.Write(mockEsServerResponse)
	}))
	defer server.Close()
	rawClient, err := elastic.NewClient(
		elastic.SetURL(server.URL),
		elastic.SetSniff(false),
	)
	require.NoError(t, err)
	t.Cleanup(rawClient.Stop)
	templateName := "jaeger-span"
	t.Run("ES 7", func(t *testing.T) {
		client := WrapESClient(rawClient, nil, 7, nil)
		service := client.GetTemplateMappings(templateName)
		mappings, err := service.Do(t.Context())
		require.NoError(t, err)
		assert.NotNil(t, mappings)
		assert.Contains(t, mappings, "properties")
	})
	t.Run("ES 8", func(t *testing.T) {
		client := WrapESClient(rawClient, nil, 8, nil)
		service := client.GetTemplateMappings(templateName)
		mappings, err := service.Do(t.Context())
		require.NoError(t, err)
		assert.NotNil(t, mappings)
		assert.Contains(t, mappings, "properties")
	})
}
