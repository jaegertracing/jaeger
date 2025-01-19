// Copyright (c) 2025 The Jaeger Authors.
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

func TestCreate(t *testing.T) {
	dataStreamName := "jaeger-span"
	tests := []struct {
		name         string
		responseCode int
		response     string
		errContains  string
	}{
		{
			name:         "success",
			responseCode: http.StatusOK,
		},
		{
			name:         "client error",
			responseCode: http.StatusBadRequest,
			response:     esErrResponse,
			errContains:  "failed to create data stream: jaeger-span",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
				assert.True(t, strings.HasSuffix(req.URL.String(), dataStreamName))
				assert.Equal(t, http.MethodPut, req.Method)
				assert.Equal(t, "Basic foobar", req.Header.Get("Authorization"))
				res.WriteHeader(test.responseCode)
				res.Write([]byte(test.response))
			}))
			defer testServer.Close()

			c := &DataStreamClient{
				Client: Client{
					Client:    testServer.Client(),
					Endpoint:  testServer.URL,
					BasicAuth: "foobar",
				},
			}
			err := c.Create(dataStreamName)
			if test.errContains != "" {
				require.ErrorContains(t, err, test.errContains)
			}
		})
	}
}
