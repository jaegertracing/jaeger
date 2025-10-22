// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package awssigv4

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoundTripper_SignsRequest(t *testing.T) {
	var capturedRequest *http.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedRequest = r
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	rt := &RoundTripper{
		Transport: http.DefaultTransport,
		Signer:    v4.NewSigner(),
		Region:    "us-east-1",
		Service:   "es",
		CredsProvider: credentials.NewStaticCredentialsProvider(
			"test-access-key",
			"test-secret-key",
			"",
		),
	}

	req, err := http.NewRequest("GET", server.URL+"/test-index/_search", nil)
	require.NoError(t, err)

	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.NotNil(t, capturedRequest)
	assert.NotEmpty(t, capturedRequest.Header.Get("Authorization"))
	assert.Contains(t, capturedRequest.Header.Get("Authorization"), "AWS4-HMAC-SHA256")
	assert.NotEmpty(t, capturedRequest.Header.Get("X-Amz-Date"))
}

func TestNewRoundTripper_WithStaticCredentials(t *testing.T) {
	rt, err := NewRoundTripper(
		http.DefaultTransport,
		"us-west-2",
		"es",
		"test-key",
		"test-secret",
		"",
		"",
	)

	require.NoError(t, err)
	assert.NotNil(t, rt)
	assert.Equal(t, "us-west-2", rt.Region)
	assert.Equal(t, "es", rt.Service)
	assert.NotNil(t, rt.Signer)
	assert.NotNil(t, rt.CredsProvider)

	creds, err := rt.CredsProvider.Retrieve(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "test-key", creds.AccessKeyID)
	assert.Equal(t, "test-secret", creds.SecretAccessKey)
}

func TestNewRoundTripper_DefaultService(t *testing.T) {
	rt, err := NewRoundTripper(
		http.DefaultTransport,
		"eu-west-1",
		"",
		"test-key",
		"test-secret",
		"",
		"",
	)

	require.NoError(t, err)
	assert.Equal(t, "es", rt.Service)
}

func TestNewRoundTripper_RequiresRegion(t *testing.T) {
	_, err := NewRoundTripper(
		http.DefaultTransport,
		"",
		"es",
		"test-key",
		"test-secret",
		"",
		"",
	)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "AWS region is required")
}

func TestRoundTripper_WithBody(t *testing.T) {
	var capturedRequest *http.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedRequest = r
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	rt := &RoundTripper{
		Transport: http.DefaultTransport,
		Signer:    v4.NewSigner(),
		Region:    "us-east-1",
		Service:   "es",
		CredsProvider: credentials.NewStaticCredentialsProvider(
			"test-access-key",
			"test-secret-key",
			"",
		),
	}

	body := strings.NewReader(`{"query":{"match_all":{}}}`)
	req, err := http.NewRequest("POST", server.URL+"/_search", body)
	require.NoError(t, err)


	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	defer resp.Body.Close()

	require.NotNil(t, capturedRequest, "Server did not capture the request")
	assert.NotEmpty(t, capturedRequest.Header.Get("Authorization"), "Authorization header should not be empty")
	assert.Contains(t, capturedRequest.Header.Get("Authorization"), "AWS4-HMAC-SHA256", "Should contain AWS4-HMAC-SHA256 signature")

}

func TestRoundTripper_NoTransport(t *testing.T) {
	rt := &RoundTripper{
		Transport: nil,
		Signer:    v4.NewSigner(),
		Region:    "us-east-1",
		Service:   "es",
		CredsProvider: credentials.NewStaticCredentialsProvider(
			"test-key",
			"test-secret",
			"",
		),
	}

	req, err := http.NewRequest("GET", "http://localhost:9200", nil)
	require.NoError(t, err)

	_, err = rt.RoundTrip(req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no http.RoundTripper provided")
}
