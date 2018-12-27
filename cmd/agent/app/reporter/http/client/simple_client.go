package client

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
)

// SimpleClient implements Client interface
type SimpleClient struct {
	Endpoint string
	Cli      *http.Client
}

// Post send payload to endpoint and handle the response
func (c *SimpleClient) Post(payload *bytes.Buffer) error {
	req, err := c.createRequest(payload)
	if err != nil {
		return nil
	}
	resp, err := c.Cli.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("failed to post payload to collector, request statusCode = %d", resp.StatusCode)
	}
	io.Copy(ioutil.Discard, resp.Body)
	defer resp.Body.Close()
	return nil
}

func (c *SimpleClient) createRequest(payload *bytes.Buffer) (*http.Request, error) {
	req, err := http.NewRequest(http.MethodPost, c.Endpoint, payload)
	if err != nil {
		return nil, err
	}
	req.Header.Set(RequestHeaderContentType, AllowedContentType)
	return req, nil
}
