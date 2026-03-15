// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type errorReadCloser struct{}

func (*errorReadCloser) Read(_ []byte) (int, error) {
	return 0, errors.New("read error")
}

func (*errorReadCloser) Close() error {
	return nil
}

type errorRoundTripper struct{}

func (*errorRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("transport error")
}

type readErrorRoundTripper struct{}

func (*readErrorRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       &errorReadCloser{},
		Header:     make(http.Header),
	}, nil
}

func TestRequest(t *testing.T) {
	tests := []struct {
		name         string
		method       string
		body         []byte
		statusCode   int
		responseBody string
		expectError  bool
		checkBody    bool
	}{
		{
			name:         "success without body",
			method:       http.MethodGet,
			statusCode:   http.StatusOK,
			responseBody: "OK",
			expectError:  false,
		},
		{
			name:         "success with body",
			method:       http.MethodPost,
			body:         []byte(`{"query": "test"}`),
			statusCode:   http.StatusOK,
			responseBody: "OK",
			expectError:  false,
			checkBody:    true,
		},
		{
			name:         "server error",
			method:       http.MethodGet,
			statusCode:   http.StatusInternalServerError,
			responseBody: "error",
			expectError:  true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, test.method, r.Method)
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

				if test.checkBody {
					body, err := io.ReadAll(r.Body)
					assert.NoError(t, err)
					assert.Equal(t, string(test.body), string(body))
				}

				w.WriteHeader(test.statusCode)
				w.Write([]byte(test.responseBody))
			}))
			defer testServer.Close()

			c := &Client{
				Endpoint: testServer.URL,
				Client:   testServer.Client(),
			}

			req := elasticRequest{
				method:   test.method,
				endpoint: "",
				body:     test.body,
			}

			resp, err := c.request(req)

			if test.expectError {
				require.Error(t, err)
				assert.Empty(t, resp)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, test.responseBody, string(resp))
		})
	}

	t.Run("client do error", func(t *testing.T) {
		c := &Client{
			Endpoint: "http://example.com",
			Client: &http.Client{
				Transport: &errorRoundTripper{},
			},
		}
		req := elasticRequest{
			method:   "GET",
			endpoint: "",
		}
		resp, err := c.request(req)
		require.Error(t, err)
		assert.Empty(t, resp)
	})
	t.Run("response body read error", func(t *testing.T) {
		c := &Client{
			Endpoint: "http://example.com",
			Client: &http.Client{
				Transport: &readErrorRoundTripper{},
			},
		}
		req := elasticRequest{
			method:   "GET",
			endpoint: "",
		}
		resp, err := c.request(req)
		require.Error(t, err)
		assert.Empty(t, resp)
	})
	t.Run("invalid endpoint url", func(t *testing.T) {
		c := &Client{
			Endpoint: "http:// invalid-url",
			Client:   http.DefaultClient,
		}
		req := elasticRequest{
			method:   "GET",
			endpoint: "",
		}
		resp, err := c.request(req)
		require.Error(t, err)
		assert.Empty(t, resp)
	})
}

func TestSetAuthorization(t *testing.T) {
	tests := []struct {
		name       string
		basicAuth  string
		expectAuth bool
	}{
		{
			name:       "basic auth present",
			basicAuth:  "dfhsfbsjfns",
			expectAuth: true,
		},
		{
			name:       "basic auth empty",
			basicAuth:  "",
			expectAuth: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "http://example.com", http.NoBody)

			c := &Client{
				BasicAuth: test.basicAuth,
			}

			c.setAuthorization(req)

			authHeader := req.Header.Get("Authorization")

			if test.expectAuth {
				assert.Equal(t, "Basic "+test.basicAuth, authHeader)
			} else {
				assert.Empty(t, authHeader)
			}
		})
	}
}

func TestHandleFailedRequest(t *testing.T) {
	tests := []struct {
		name                string
		body                io.ReadCloser
		statusCode          int
		expectedError       string
		expectResponseError bool
	}{
		{
			name:                "body present",
			body:                io.NopCloser(strings.NewReader("failure")),
			statusCode:          http.StatusInternalServerError,
			expectedError:       "request failed",
			expectResponseError: true,
		},
		{
			name:                "body nil",
			body:                nil,
			statusCode:          http.StatusBadRequest,
			expectedError:       "request failed",
			expectResponseError: true,
		},
		{
			name:                "body read error",
			body:                &errorReadCloser{},
			statusCode:          http.StatusInternalServerError,
			expectedError:       "failed to read response body",
			expectResponseError: false,
		},
	}

	c := &Client{}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			res := &http.Response{
				StatusCode: test.statusCode,
				Body:       test.body,
			}

			err := c.handleFailedRequest(res)

			require.ErrorContains(t, err, test.expectedError)

			var respErr ResponseError
			if test.expectResponseError {
				require.ErrorAs(t, err, &respErr)
				assert.Equal(t, test.statusCode, respErr.StatusCode)
				if test.body == nil {
					assert.Empty(t, respErr.Body)
				} else if test.name == "body present" {
					assert.Equal(t, []byte("failure"), respErr.Body)
					assert.Contains(t, err.Error(), string(respErr.Body))
				} else if test.name == "body read error" {
					assert.Nil(t, respErr.Body)
				}
			}
		})
	}
}
