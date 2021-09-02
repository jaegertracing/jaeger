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
	"io/ioutil"
	"net/http"
)

type ResponseError struct {
	Err        error
	StatusCode int
	Body       []byte
}

func (r ResponseError) Error() string {
	return r.Err.Error()
}

func NewResponseError(err error, code int, body []byte) ResponseError {
	return ResponseError{
		Err:        err,
		StatusCode: code,
		Body:       body,
	}
}

type Client struct {
	// Http client.
	Client *http.Client
	// ES server endpoint.
	Endpoint string
	// Basic authentication string.
	BasicAuth string
}

func (c *Client) getRequest(endpoint string) ([]byte, error) {
	r, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/%s", c.Endpoint, endpoint), nil)
	if err != nil {
		return []byte{}, err
	}
	c.setAuthorization(r)
	res, err := c.Client.Do(r)
	if err != nil {
		return []byte{}, err
	}
	if res.StatusCode != http.StatusOK {
		return []byte{}, c.handleFailedRequest(res)
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return []byte{}, err
	}
	return body, nil
}

func (c *Client) putRequest(endpoint string, body []byte) error {
	var reader *bytes.Buffer
	var r *http.Request
	var err error
	if body != nil {
		reader = bytes.NewBuffer(body)
		r, err = http.NewRequest(http.MethodPut, fmt.Sprintf("%s/%s", c.Endpoint, endpoint), reader)
	} else {
		r, err = http.NewRequest(http.MethodPut, fmt.Sprintf("%s/%s", c.Endpoint, endpoint), nil)

	}
	if err != nil {
		return err
	}
	c.setAuthorization(r)
	r.Header.Add("Content-Type", "application/json")
	res, err := c.Client.Do(r)
	if err != nil {
		return err
	}
	if res.StatusCode != http.StatusOK {
		return c.handleFailedRequest(res)
	}
	return nil
}

func (c *Client) deleteRequest(endpoint string, body []byte) error {
	r, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/%s", c.Endpoint, endpoint), nil)
	if err != nil {
		return err
	}
	c.setAuthorization(r)
	r.Header.Add("Content-Type", "application/json")
	res, err := c.Client.Do(r)
	if err != nil {
		return err
	}
	if res.StatusCode != http.StatusOK {
		return c.handleFailedRequest(res)
	}
	return nil
}

func (c *Client) postRequest(endpoint string, body []byte) error {
	var reader *bytes.Buffer
	var r *http.Request
	var err error
	if body != nil {
		reader = bytes.NewBuffer(body)
		r, err = http.NewRequest(http.MethodPost, fmt.Sprintf("%s/%s", c.Endpoint, endpoint), reader)
	} else {
		r, err = http.NewRequest(http.MethodPost, fmt.Sprintf("%s/%s", c.Endpoint, endpoint), nil)
	}
	if err != nil {
		return err
	}
	c.setAuthorization(r)
	r.Header.Add("Content-Type", "application/json")
	res, err := c.Client.Do(r)
	if err != nil {
		return err
	}
	if res.StatusCode != http.StatusOK {
		return c.handleFailedRequest(res)
	}
	return nil
}

func (c *Client) setAuthorization(r *http.Request) {
	if c.BasicAuth != "" {
		r.Header.Add("Authorization", fmt.Sprintf("Basic %s", c.BasicAuth))
	}
}

func (c *Client) handleFailedRequest(res *http.Response) error {
	if res.Body != nil {
		bodyBytes, err := ioutil.ReadAll(res.Body)
		if err != nil {
			return NewResponseError(fmt.Errorf("request failed and failed to read response body, status code: %d, %w", res.StatusCode, err), res.StatusCode, nil)
		}
		body := string(bodyBytes)
		return NewResponseError(fmt.Errorf("request failed, status code: %d, body: %s", res.StatusCode, body), res.StatusCode, bodyBytes)

	}
	return NewResponseError(fmt.Errorf("request failed, status code: %d", res.StatusCode), res.StatusCode, nil)
}
