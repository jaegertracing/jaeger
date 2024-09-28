// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configopaque"
)

func TestResponseHeadersHandler(t *testing.T) {
	responseHeaders := make(map[string]configopaque.String)
	responseHeaders["Access-Control-Allow-Origin"] = "https://mozilla.org"
	responseHeaders["Access-Control-Expose-Headers"] = "X-My-Custom-Header"
	responseHeaders["Access-Control-Expose-Headers"] = "X-Another-Custom-Header"
	responseHeaders["Access-Control-Request-Headers"] = "field1, field2"

	emptyHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte{})
	})

	handler := responseHeadersHandler(emptyHandler, responseHeaders)
	server := httptest.NewServer(handler)
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL, nil)
	require.NoError(t, err)

	resp, err := server.Client().Do(req)
	require.NoError(t, err)

	for k, v := range responseHeaders {
		assert.Equal(t, []string{v.String()}, resp.Header[k])
	}
}
