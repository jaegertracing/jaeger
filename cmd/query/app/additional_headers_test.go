// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdditionalHeadersHandler(t *testing.T) {
	additionalHeaders := http.Header{}
	additionalHeaders.Add("Access-Control-Allow-Origin", "https://mozilla.org")
	additionalHeaders.Add("Access-Control-Expose-Headers", "X-My-Custom-Header")
	additionalHeaders.Add("Access-Control-Expose-Headers", "X-Another-Custom-Header")
	additionalHeaders.Add("Access-Control-Request-Headers", "field1, field2")

	emptyHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte{})
	})

	handler := additionalHeadersHandler(emptyHandler, additionalHeaders)
	server := httptest.NewServer(handler)
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL, nil)
	require.NoError(t, err)

	resp, err := server.Client().Do(req)
	require.NoError(t, err)

	for k, v := range additionalHeaders {
		assert.Equal(t, v, resp.Header[k])
	}
}
