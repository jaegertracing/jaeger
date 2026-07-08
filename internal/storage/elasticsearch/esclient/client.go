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

	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
)

// Client executes requests against Elasticsearch/OpenSearch using direct HTTP
// calls (no official Go client) over the shared transport pool.
type Client struct {
	transport *rawClient
	timeout   time.Duration

	// version is the backend version (Elasticsearch/OpenSearch), resolved once by
	// NewClient (from an explicit config.Version, else a single probe of the
	// cluster). Sub-clients read it internally to pick flavor-dependent endpoints
	// (CreateTemplate, ILM vs ISM) without re-probing per call. It is unexported
	// and has no accessor, so it stays encapsulated — business logic never sees a
	// BackendVersion.
	version es.BackendVersion
}

// NewClient builds a Client that sends requests across c.Servers through the shared
// transport pool. Its base RoundTripper is the full stack from GetHTTPRoundTripper
// (TLS, basic/bearer/API-key, custom headers, and — when httpAuth is non-nil —
// SigV4), so every request carries the configured auth and headers.
// c.QueryTimeout bounds each request (0 means no bound).
//
// The backend version is resolved here, at construction: NewClient honors an
// explicit c.Version, otherwise it probes the cluster once. Every returned
// Client therefore carries a resolved version, so version-dependent operations
// never re-probe and business logic never handles a BackendVersion.
func NewClient(ctx context.Context, c *config.Configuration, logger *zap.Logger, httpAuth extensionauth.HTTPClient) (Client, error) {
	base, err := GetHTTPRoundTripper(ctx, c, logger, httpAuth)
	if err != nil {
		return Client{}, err
	}
	transport, err := newRawClient(c.Servers, base)
	if err != nil {
		return Client{}, err
	}
	client := Client{transport: transport, timeout: c.QueryTimeout}
	// Honor an explicit c.Version, otherwise probe the cluster once via the
	// low-level ping. Both planes share es.ResolveBackendVersion.
	version, err := es.ResolveBackendVersion(ctx, c.Version, client.ping)
	if err != nil {
		return Client{}, fmt.Errorf("failed to resolve backend version: %w", err)
	}
	client.version = version
	return client, nil
}

// Close releases the client's pooled idle connections. It is safe to call on a
// zero Client. The transport has no background goroutines (node discovery is off),
// so there is nothing else to stop.
func (c Client) Close() error {
	if c.transport != nil {
		c.transport.close()
	}
	return nil
}

type elasticRequest struct {
	endpoint string
	body     []byte
	method   string
	// contentType overrides the request Content-Type; empty defaults to
	// application/json. The _bulk NDJSON path sets application/x-ndjson.
	contentType string
}

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

// Perform sends a fully-formed HTTP request through the client's transport — the
// shared multi-node pool with the auth/TLS/SigV4 RoundTripper stack — and returns
// the raw response. It delegates down through rawClient rather than exposing the
// pool, so transport-layer behavior applies. It satisfies go-elasticsearch's
// esapi.Transport, which lets the official esutil.BulkIndexer run over our
// transport instead of a product-checked go-elasticsearch client. Unlike request,
// Perform neither builds the request nor interprets the status code — the caller
// owns those (esutil writes and parses the _bulk protocol itself).
func (c Client) Perform(req *http.Request) (*http.Response, error) {
	// A caller may submit a request with no deadline — esutil.BulkIndexer builds
	// its _bulk flushes (and its shutdown Close flush) from a background context.
	// Apply the client's QueryTimeout so those writes can't hang forever, the way
	// an http.Client.Timeout bounds every request. request() already sets its own
	// deadline, so this only fills the gap when one is absent.
	if c.timeout <= 0 {
		return c.transport.perform(req)
	}
	if _, hasDeadline := req.Context().Deadline(); hasDeadline {
		return c.transport.perform(req)
	}
	// The timeout must cover the whole response read (like http.Client.Timeout), so
	// the cancel fires when the body is closed rather than when Perform returns —
	// the caller reads the body afterwards.
	ctx, cancel := context.WithTimeout(req.Context(), c.timeout)
	res, err := c.transport.perform(req.WithContext(ctx))
	if err != nil {
		cancel()
		return nil, err
	}
	res.Body = &cancelOnCloseReader{ReadCloser: res.Body, cancel: cancel}
	return res, nil
}

// cancelOnCloseReader cancels a context once the wrapped body is closed. Perform
// uses it so a deadline it applies is released only when the caller finishes
// reading, instead of the moment Perform returns (which would abort the read).
type cancelOnCloseReader struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (r *cancelOnCloseReader) Close() error {
	err := r.ReadCloser.Close()
	r.cancel()
	return err
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
	contentType := esRequest.contentType
	if contentType == "" {
		contentType = "application/json"
	}
	r.Header.Add("Content-Type", contentType)
	res, err := c.Perform(r)
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

func (*Client) handleFailedRequest(res *http.Response) ResponseError {
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
