// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateComposableTemplate(t *testing.T) {
	const (
		name = "jaeger-spans"
		body = "template content"
	)
	tests := []struct {
		name         string
		call         func(context.Context, IndicesClient) error
		wantEndpoint string
		responseCode int
		response     string
		errContains  string
	}{
		{
			name:         "component template/200",
			call:         func(ctx context.Context, c IndicesClient) error { return c.CreateComponentTemplate(ctx, name, body) },
			wantEndpoint: "/_component_template/" + name,
			responseCode: http.StatusOK,
		},
		{
			name:         "component template/201",
			call:         func(ctx context.Context, c IndicesClient) error { return c.CreateComponentTemplate(ctx, name, body) },
			wantEndpoint: "/_component_template/" + name,
			responseCode: http.StatusCreated,
		},
		{
			name:         "index template/200",
			call:         func(ctx context.Context, c IndicesClient) error { return c.CreateIndexTemplate(ctx, name, body) },
			wantEndpoint: "/_index_template/" + name,
			responseCode: http.StatusOK,
		},
		{
			name:         "index template/201",
			call:         func(ctx context.Context, c IndicesClient) error { return c.CreateIndexTemplate(ctx, name, body) },
			wantEndpoint: "/_index_template/" + name,
			responseCode: http.StatusCreated,
		},
		{
			name:         "component template/error",
			call:         func(ctx context.Context, c IndicesClient) error { return c.CreateComponentTemplate(ctx, name, body) },
			wantEndpoint: "/_component_template/" + name,
			responseCode: http.StatusBadRequest,
			response:     esErrResponse,
			errContains:  "failed to create component template: " + name,
		},
		{
			name:         "index template/error",
			call:         func(ctx context.Context, c IndicesClient) error { return c.CreateIndexTemplate(ctx, name, body) },
			wantEndpoint: "/_index_template/" + name,
			responseCode: http.StatusInternalServerError,
			response:     esErrResponse,
			errContains:  "failed to create index template: " + name,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
				assert.Equal(t, test.wantEndpoint, req.URL.String())
				assert.Equal(t, http.MethodPut, req.Method)
				assert.Equal(t, "Basic foobar", req.Header.Get("Authorization"))
				reqBody, err := io.ReadAll(req.Body)
				assert.NoError(t, err)
				assert.Equal(t, body, string(reqBody))

				res.WriteHeader(test.responseCode)
				res.Write([]byte(test.response))
			}))
			defer testServer.Close()

			c := IndicesClient{
				Client: Client{
					Client:    testServer.Client(),
					Endpoint:  testServer.URL,
					BasicAuth: "foobar",
				},
			}
			err := test.call(context.Background(), c)
			if test.errContains != "" {
				assert.ErrorContains(t, err, test.errContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCreateComposableTemplate_TransportError(t *testing.T) {
	// A closed server forces a transport error (not a ResponseError), exercising
	// the non-HTTP error path.
	testServer := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	c := IndicesClient{Client: Client{Client: testServer.Client(), Endpoint: testServer.URL}}
	testServer.Close()

	err := c.CreateComponentTemplate(context.Background(), "jaeger-spans", "body")
	require.ErrorContains(t, err, "failed to create component template: jaeger-spans")
}
