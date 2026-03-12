package client

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
			method:       "GET",
			statusCode:   http.StatusOK,
			responseBody: "OK",
			expectError:  false,
		},
		{
			name:         "success with body",
			method:       "POST",
			body:         []byte(`{"query": "test"}`),
			statusCode:   http.StatusOK,
			responseBody: "OK",
			expectError:  false,
			checkBody:    true,
		},
		{
			name:         "server error",
			method:       "GET",
			statusCode:   http.StatusInternalServerError,
			responseBody: "error",
			expectError:  true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, test.method, r.Method)
				assert.Equal(t, "application/json", r.Header.Get("Content-type"))

				if test.checkBody {
					body, err := io.ReadAll(r.Body)
					require.NoError(t, err)
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

			req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)

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
		name       string
		body       io.ReadCloser
		statusCode int
	}{
		{
			name:       "body present",
			body:       io.NopCloser(strings.NewReader("failure")),
			statusCode: http.StatusInternalServerError,
		},
		{
			name:       "body nil",
			body:       nil,
			statusCode: http.StatusBadRequest,
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

			require.Error(t, err)
			assert.Contains(t, err.Error(), "request failed")
		})
	}
}
