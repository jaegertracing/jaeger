// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package estesting

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWriteMockMappingResponse(t *testing.T) {
	tests := []struct {
		name           string
		path           string
		expectedResult bool
		expectedBody   bool
		contains       string
	}{
		{
			name:           "contains _template",
			path:           "/some_template/path",
			expectedResult: true,
			expectedBody:   true,
			contains:       "jaeger-span",
		},
		{
			name:           "contains _index_template",
			path:           "/some_index_template/path",
			expectedResult: true,
			expectedBody:   true,
			contains:       "index_templates",
		},
		{
			name:           "does not contain _template",
			path:           "/some/other/path",
			expectedResult: false,
			expectedBody:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, tt.path, http.NoBody)

			result := WriteMockMappingResponse(w, r)

			assert.Equal(t, tt.expectedResult, result)
			if tt.expectedBody {
				assert.NotEmpty(t, w.Body.String())
				assert.Contains(t, w.Body.String(), tt.contains)
			} else {
				assert.Empty(t, w.Body.String())
			}
		})
	}
}
