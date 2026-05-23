// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package eswrapper

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/olivere/elastic/v7"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/estesting"
)

var mockEsServerResponse = []byte(`
{
	"Version": {
		"Number": "7"
	}
}
`)

func TestClientWrapper_GetTemplateMappings(t *testing.T) {
	templateName := "jaeger-span"

	tests := []struct {
		name          string
		esVersion     uint
		handler       http.HandlerFunc
		expectedError string
	}{
		{
			name:      "ES 7 - Success",
			esVersion: 7,
			handler: func(w http.ResponseWriter, r *http.Request) {
				estesting.WriteMockMappingResponse(w, r)
			},
		},
		{
			name:      "ES 7 - Server Error",
			esVersion: 7,
			handler: func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "_template") {
					w.WriteHeader(http.StatusInternalServerError)
				}
			},
			expectedError: "elastic: Error 500 (Internal Server Error)",
		},
		{
			name:      "ES 7 - Template Not Found",
			esVersion: 7,
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Write([]byte(`{"other-template": {"mappings": {}}}`))
			},
			expectedError: "template jaeger-span not found",
		},
		{
			name:      "ES 8 - Success",
			esVersion: 8,
			handler: func(w http.ResponseWriter, r *http.Request) {
				estesting.WriteMockMappingResponse(w, r)
			},
		},
		{
			name:      "ES 8 - Server Error",
			esVersion: 8,
			handler: func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "_index_template") {
					w.WriteHeader(http.StatusInternalServerError)
				}
			},
			expectedError: "elastic: Error 500 (Internal Server Error)",
		},
		{
			name:      "ES 8 - Template Not Found",
			esVersion: 8,
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Write([]byte(`{"index_templates": [{"name": "other-template", "index_template": {"template": {"mappings": {}}}}]}`))
			},
			expectedError: "template jaeger-span not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/" {
					w.Write(mockEsServerResponse)
					return
				}
				tt.handler(w, r)
			}))
			defer server.Close()

			rawClient, err := elastic.NewClient(
				elastic.SetURL(server.URL),
				elastic.SetSniff(false),
			)
			require.NoError(t, err)
			t.Cleanup(rawClient.Stop)

			client := WrapESClient(rawClient, nil, tt.esVersion, nil)
			service := client.GetTemplateMappings(templateName)

			mappings, err := service.Do(context.Background())

			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				assert.Nil(t, mappings)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, mappings)
				assert.Contains(t, mappings, "properties")
			}
		})
	}
}
