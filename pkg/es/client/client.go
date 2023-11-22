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
	"bytes"
	"fmt"
	"io"
	"net/http"
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

// Client executes requests against Elasticsearch using direct HTTP calls,
// without using the official Go client for ES.
type Client struct {
	// Http client.
	Client *http.Client
	// ES server endpoint.
	Endpoint string
	// Basic authentication string.
	BasicAuth string
}

type elasticRequest struct {
	endpoint string
	body     []byte
	method   string
}

func (c *Client) request(esRequest elasticRequest) ([]byte, error) {
	var reader *bytes.Buffer
	var r *http.Request
	var err error
	if len(esRequest.body) > 0 {
		reader = bytes.NewBuffer(esRequest.body)
		r, err = http.NewRequest(esRequest.method, fmt.Sprintf("%s/%s", c.Endpoint, esRequest.endpoint), reader)
	} else {
		r, err = http.NewRequest(esRequest.method, fmt.Sprintf("%s/%s", c.Endpoint, esRequest.endpoint), nil)
	}
	if err != nil {
		return []byte{}, err
	}
	c.setAuthorization(r)
	r.Header.Add("Content-Type", "application/json")
	res, err := c.Client.Do(r)
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

func (c *Client) setAuthorization(r *http.Request) {
	if c.BasicAuth != "" {
		r.Header.Add("Authorization", fmt.Sprintf("Basic %s", c.BasicAuth))
	}
}

func (c *Client) handleFailedRequest(res *http.Response) error {
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
