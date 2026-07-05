// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package esclient

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.opentelemetry.io/collector/extension/extensionauth"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
)

// ResponseError holds information about a request error
type ResponseError struct {
	// Error returned by the http client
	Err error
	// StatusCode is the http code returned by the server (if any)
	StatusCode int
	// Body is the bytes readed in the response (if any)
	Body []byte
}

// Error returns the error string of the Err field
func (r ResponseError) Error() string {
	return r.Err.Error()
}

func (r ResponseError) prefixMessage(message string) ResponseError {
	return ResponseError{
		Err:        fmt.Errorf("%s, %w", message, r.Err),
		StatusCode: r.StatusCode,
		Body:       r.Body,
	}
}

func newResponseError(err error, code int, body []byte) ResponseError {
	return ResponseError{
		Err:        err,
		StatusCode: code,
		Body:       body,
	}
}

// Client executes requests against Elasticsearch/OpenSearch using direct HTTP
// calls (no official Go client) over the shared transport pool.
type Client struct {
	transport *rawClient
	timeout   time.Duration
}

// NewClient builds a Client that sends requests across c.Servers through the shared
// transport pool. Its base RoundTripper is the full stack from GetHTTPRoundTripper
// (TLS, basic/bearer/API-key, custom headers, and — when httpAuth is non-nil —
// SigV4), so every request carries the configured auth and headers.
// c.QueryTimeout bounds each request (0 means no bound).
func NewClient(ctx context.Context, c *config.Configuration, logger *zap.Logger, httpAuth extensionauth.HTTPClient) (Client, error) {
	base, err := GetHTTPRoundTripper(ctx, c, logger, httpAuth)
	if err != nil {
		return Client{}, err
	}
	transport, err := newRawClient(c.Servers, base)
	if err != nil {
		return Client{}, err
	}
	return Client{transport: transport, timeout: c.QueryTimeout}, nil
}

type elasticRequest struct {
	endpoint string
	body     []byte
	method   string
}

func (c *Client) request(ctx context.Context, esRequest elasticRequest) ([]byte, error) {
	if c.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}
	// A relative path: the pool selects a node and fills in its scheme and host.
	var reqBody io.Reader = http.NoBody
	if len(esRequest.body) > 0 {
		reqBody = bytes.NewBuffer(esRequest.body)
	}
	r, err := http.NewRequestWithContext(ctx, esRequest.method, "/"+esRequest.endpoint, reqBody)
	if err != nil {
		return []byte{}, err
	}
	r.Header.Add("Content-Type", "application/json")
	res, err := c.transport.perform(r)
	if err != nil {
		return []byte{}, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return []byte{}, c.handleFailedRequest(res)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return []byte{}, err
	}
	return body, nil
}

func (*Client) handleFailedRequest(res *http.Response) error {
	if res.Body != nil {
		bodyBytes, err := io.ReadAll(res.Body)
		if err != nil {
			return newResponseError(fmt.Errorf("request failed and failed to read response body, status code: %d, %w", res.StatusCode, err), res.StatusCode, nil)
		}
		body := string(bodyBytes)
		return newResponseError(fmt.Errorf("request failed, status code: %d, body: %s", res.StatusCode, body), res.StatusCode, bodyBytes)
	}
	return newResponseError(fmt.Errorf("request failed, status code: %d", res.StatusCode), res.StatusCode, nil)
}
