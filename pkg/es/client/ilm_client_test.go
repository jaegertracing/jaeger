// Copyright (c) 2021 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
				require.Error(t, err)
				assert.Contains(t, err.Error(), test.errContains)
			}
			assert.Equal(t, test.expectedResult, result)
		})
	}
}
