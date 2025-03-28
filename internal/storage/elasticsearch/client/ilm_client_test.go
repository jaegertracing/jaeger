// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExists(t *testing.T) {
	tests := []struct {
		name           string
		responseCode   int
		response       string
		errContains    string
		expectedResult bool
	}{
		{
			name:           "found",
			responseCode:   http.StatusOK,
			expectedResult: true,
		},
		{
			name:         "not found",
			responseCode: http.StatusNotFound,
			response:     esErrResponse,
		},
		{
			name:         "client error",
			responseCode: http.StatusBadRequest,
			response:     esErrResponse,
			errContains:  "failed to get ILM policy: jaeger-ilm-policy",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
				assert.True(t, strings.HasSuffix(req.URL.String(), "_ilm/policy/jaeger-ilm-policy"))
				assert.Equal(t, http.MethodGet, req.Method)
				assert.Equal(t, "Basic foobar", req.Header.Get("Authorization"))
				res.WriteHeader(test.responseCode)
				res.Write([]byte(test.response))
			}))
			defer testServer.Close()

			c := &ILMClient{
				Client: Client{
					Client:    testServer.Client(),
					Endpoint:  testServer.URL,
					BasicAuth: "foobar",
				},
			}
			result, err := c.Exists("jaeger-ilm-policy")
			if test.errContains != "" {
				require.ErrorContains(t, err, test.errContains)
			}
			assert.Equal(t, test.expectedResult, result)
		})
	}
}
