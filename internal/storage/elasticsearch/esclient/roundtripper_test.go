// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package esclient

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/config/configtls"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/auth"
	"github.com/jaegertracing/jaeger/internal/auth/bearertoken"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
)

// bearerAuth creates bearer token authentication component
func bearerAuth(filePath string, allowFromContext bool) configoptional.Optional[config.TokenAuthentication] {
	return configoptional.Some(config.TokenAuthentication{
		FilePath:         filePath,
		AllowFromContext: allowFromContext,
	})
}

func TestGetHTTPRoundTripper(t *testing.T) {
	tmpDir := t.TempDir()
	bearerTokenFile := filepath.Join(tmpDir, "bearertoken")
	require.NoError(t, os.WriteFile(bearerTokenFile, []byte("file-bearer-token"), 0o600))

	tests := []struct {
		name            string
		cfg             *config.Configuration
		ctx             context.Context
		wantErrContains string
		validate        func(t *testing.T, rt http.RoundTripper)
	}{
		{
			name: "Secure mode without auth",
			cfg: &config.Configuration{
				TLS: configtls.ClientConfig{Insecure: false},
			},
			ctx: context.Background(),
			validate: func(t *testing.T, rt http.RoundTripper) {
				assert.NotNil(t, rt)
				_, ok := rt.(*auth.RoundTripper)
				assert.False(t, ok, "Should not be an auth round tripper")
			},
		},
		{
			name: "Insecure mode without auth",
			cfg: &config.Configuration{
				TLS: configtls.ClientConfig{Insecure: true},
			},
			ctx: context.Background(),
			validate: func(t *testing.T, rt http.RoundTripper) {
				assert.NotNil(t, rt)
				transport, ok := rt.(*http.Transport)
				require.True(t, ok)
				assert.True(t, transport.TLSClientConfig.InsecureSkipVerify)
			},
		},
		{
			name: "Bearer auth not applicable (empty config)",
			cfg: &config.Configuration{
				TLS: configtls.ClientConfig{Insecure: true},
				Authentication: config.Authentication{
					BearerTokenAuth: bearerAuth("", false),
				},
			},
			ctx: context.Background(),
			validate: func(t *testing.T, rt http.RoundTripper) {
				assert.NotNil(t, rt)
				// Should be plain transport since auth is not applicable
				_, ok := rt.(*auth.RoundTripper)
				assert.False(t, ok, "Should not be an auth round tripper when config is not applicable")

				transport, ok := rt.(*http.Transport)
				assert.True(t, ok, "Should be plain http.Transport")
				assert.True(t, transport.TLSClientConfig.InsecureSkipVerify)
			},
		},
		{
			name: "Secure mode with bearer token from file",
			cfg: &config.Configuration{
				TLS: configtls.ClientConfig{Insecure: false},
				Authentication: config.Authentication{
					BearerTokenAuth: bearerAuth(bearerTokenFile, false),
				},
			},
			ctx: context.Background(),
			validate: func(t *testing.T, rt http.RoundTripper) {
				assert.NotNil(t, rt)
				authRT, ok := rt.(*auth.RoundTripper)
				require.True(t, ok, "Should be an auth round tripper")
				require.Len(t, authRT.Auths, 1)
				assert.Equal(t, "Bearer", authRT.Auths[0].Scheme)
				assert.NotNil(t, authRT.Auths[0].TokenFn)
				assert.Equal(t, "file-bearer-token", authRT.Auths[0].TokenFn())
			},
		},
		{
			name: "Insecure mode with bearer token from context",
			cfg: &config.Configuration{
				TLS: configtls.ClientConfig{Insecure: true},
				Authentication: config.Authentication{
					BearerTokenAuth: bearerAuth("", true),
				},
			},
			ctx: bearertoken.ContextWithBearerToken(context.Background(), "context-bearer-token"),
			validate: func(t *testing.T, rt http.RoundTripper) {
				assert.NotNil(t, rt)
				authRT, ok := rt.(*auth.RoundTripper)
				require.True(t, ok, "Should be an auth round tripper")
				require.Len(t, authRT.Auths, 1)
				assert.Equal(t, "Bearer", authRT.Auths[0].Scheme)
				assert.NotNil(t, authRT.Auths[0].FromCtx)

				transport, ok := authRT.Transport.(*http.Transport)
				require.True(t, ok)
				assert.True(t, transport.TLSClientConfig.InsecureSkipVerify)
			},
		},
		{
			name: "BearerToken file error",
			cfg: &config.Configuration{
				Authentication: config.Authentication{
					BearerTokenAuth: bearerAuth("/does/not/exist/token", false),
				},
			},
			ctx:             context.Background(),
			wantErrContains: "no such file or directory",
		},
		{
			name: "Invalid TLS config should fail",
			cfg: &config.Configuration{
				TLS: configtls.ClientConfig{
					Insecure: false,
					Config: configtls.Config{
						CAFile: "/does/not/exist/ca.pem",
					},
				},
			},
			ctx:             context.Background(),
			wantErrContains: "failed to load TLS config",
		},
	}

	logger := zap.NewNop()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, err := GetHTTPRoundTripper(tt.ctx, tt.cfg, logger, nil)
			if tt.wantErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrContains)
				assert.Nil(t, rt)
			} else {
				require.NoError(t, err)
				tt.validate(t, rt)
			}
		})
	}
}

// Test GetHTTPRoundTripper with httpAuth error
func TestGetHTTPRoundTripperWithHTTPAuthError(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()
	// Create a mock httpAuth that will fail on RoundTripper wrapping
	mockAuth := &mockFailingHTTPAuth{}

	c := &config.Configuration{
		Servers:  []string{"http://localhost:9200"},
		LogLevel: "error",
		TLS:      configtls.ClientConfig{Insecure: true},
	}

	_, err := GetHTTPRoundTripper(ctx, c, logger, mockAuth)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to wrap round tripper with HTTP authenticator")
}

// Mock failing HTTP authenticator
type mockFailingHTTPAuth struct{}

func (*mockFailingHTTPAuth) RoundTripper(_ http.RoundTripper) (http.RoundTripper, error) {
	return nil, errors.New("mock authenticator error")
}

func TestGetHTTPRoundTripperWrappingError(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	// Create a mock failing HTTP authenticator
	mockAuth := &mockFailingHTTPAuthWrapper{}

	c := &config.Configuration{
		Servers:  []string{"http://localhost:9200"},
		LogLevel: "error",
		TLS:      configtls.ClientConfig{Insecure: true},
	}

	_, err := GetHTTPRoundTripper(ctx, c, logger, mockAuth)
	require.Error(t, err)
	require.ErrorContains(t, err, "failed to wrap round tripper with HTTP authenticator")
}

// mockFailingHTTPAuthWrapper mocks a failing HTTP authenticator for wrapping tests
type mockFailingHTTPAuthWrapper struct{}

func (*mockFailingHTTPAuthWrapper) RoundTripper(_ http.RoundTripper) (http.RoundTripper, error) {
	return nil, errors.New("wrapping error")
}

// Test GetHTTPRoundTripper with successful httpAuth wrapping
func TestGetHTTPRoundTripperWithHTTPAuthSuccess(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	// Create a mock httpAuth that will succeed
	mockAuth := &mockSuccessfulHTTPAuth{}

	c := &config.Configuration{
		Servers:  []string{"http://localhost:9200"},
		LogLevel: "error",
		TLS:      configtls.ClientConfig{Insecure: true},
	}

	rt, err := GetHTTPRoundTripper(ctx, c, logger, mockAuth)

	require.NoError(t, err)
	require.NotNil(t, rt)
	// getBodyFixRoundTripper must be outermost so that it populates
	// req.GetBody before the authenticator hashes the payload.
	bodyFixRT, ok := rt.(*getBodyFixRoundTripper)
	require.True(t, ok, "outermost round tripper should be getBodyFixRoundTripper")
	wrappedRT, ok := bodyFixRT.base.(*mockWrappedRoundTripper)
	require.True(t, ok, "authenticator wrapper should be inside getBodyFixRoundTripper")
	require.NotNil(t, wrappedRT)
}

// Mock successful HTTP authenticator
type mockSuccessfulHTTPAuth struct{}

func (*mockSuccessfulHTTPAuth) RoundTripper(rt http.RoundTripper) (http.RoundTripper, error) {
	return &mockWrappedRoundTripper{base: rt}, nil
}

// Mock wrapped round tripper
type mockWrappedRoundTripper struct {
	base http.RoundTripper
}

func (m *mockWrappedRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.base.RoundTrip(req)
}

// getBodyRecordingAuth mimics an authenticator like sigv4auth: its
// RoundTripper inspects req.GetBody at the start of RoundTrip (where
// sigv4auth hashes the payload) before delegating to the base transport.
type getBodyRecordingAuth struct {
	sawGetBody bool
}

func (a *getBodyRecordingAuth) RoundTripper(base http.RoundTripper) (http.RoundTripper, error) {
	return &getBodyRecordingRoundTripper{auth: a, base: base}, nil
}

type getBodyRecordingRoundTripper struct {
	auth *getBodyRecordingAuth
	base http.RoundTripper
}

func (rt *getBodyRecordingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.auth.sawGetBody = req.GetBody != nil
	// Short-circuit instead of hitting the network.
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
}

// TestGetHTTPRoundTripper_AuthSeesGetBody verifies that an HTTP
// authenticator sees a populated req.GetBody at signing time for requests
// where the client set req.Body without GetBody (as olivere/elastic does).
// If getBodyFixRoundTripper is wrapped inside the authenticator instead of
// outside, the authenticator hashes an empty payload and AWS-managed
// OpenSearch rejects every body-bearing request with HTTP 403.
func TestGetHTTPRoundTripper_AuthSeesGetBody(t *testing.T) {
	recordingAuth := &getBodyRecordingAuth{}

	c := &config.Configuration{
		Servers:  []string{"http://localhost:9200"},
		LogLevel: "error",
		TLS:      configtls.ClientConfig{Insecure: true},
	}

	rt, err := GetHTTPRoundTripper(context.Background(), c, zap.NewNop(), recordingAuth)
	require.NoError(t, err)

	// Mimic olivere/elastic: Body set directly, GetBody left nil.
	req, err := http.NewRequest(http.MethodPut, "http://localhost:9200/_index_template/test", http.NoBody)
	require.NoError(t, err)
	req.Body = io.NopCloser(bytes.NewReader([]byte(`{"index_patterns":["jaeger-*"]}`)))
	req.GetBody = nil

	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	assert.True(t, recordingAuth.sawGetBody,
		"authenticator must see req.GetBody populated at signing time")
}

func TestInitAuthNilInputs(t *testing.T) {
	logger := zap.NewNop()

	method, err := initBearerAuth(nil, logger)
	require.NoError(t, err)
	assert.Nil(t, method)

	method, err = initAPIKeyAuth(nil, logger)
	require.NoError(t, err)
	assert.Nil(t, method)

	method, err = initBasicAuth(nil, logger)
	require.NoError(t, err)
	assert.Nil(t, method)

	// Empty username returns nil
	method, err = initBasicAuth(&config.BasicAuthentication{Username: ""}, logger)
	require.NoError(t, err)
	assert.Nil(t, method)
}

type recordingRoundTripper struct {
	req *http.Request
}

func (r *recordingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	r.req = req
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
}

func TestGetBodyFixRoundTripper_SetsGetBody(t *testing.T) {
	payload := []byte(`{"action":"index","data":"test"}`)
	req, err := http.NewRequest(http.MethodPost, "http://localhost", http.NoBody)
	require.NoError(t, err)
	req.Body = io.NopCloser(bytes.NewReader(payload))
	req.GetBody = nil

	recorder := &recordingRoundTripper{}
	rt := &getBodyFixRoundTripper{base: recorder}

	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	require.NotNil(t, recorder.req.GetBody)
	body, err := recorder.req.GetBody()
	require.NoError(t, err)
	got, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.Equal(t, payload, got)

	sentBody, err := io.ReadAll(recorder.req.Body)
	require.NoError(t, err)
	assert.Equal(t, payload, sentBody)
}

func TestGetBodyFixRoundTripper_PreservesExistingGetBody(t *testing.T) {
	original := []byte("original")
	customGetBody := func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(original)), nil
	}

	req, err := http.NewRequest(http.MethodPost, "http://localhost", bytes.NewReader([]byte("different")))
	require.NoError(t, err)
	req.GetBody = customGetBody

	recorder := &recordingRoundTripper{}
	rt := &getBodyFixRoundTripper{base: recorder}

	_, err = rt.RoundTrip(req)
	require.NoError(t, err)

	body, err := recorder.req.GetBody()
	require.NoError(t, err)
	got, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.Equal(t, original, got)
}

func TestGetBodyFixRoundTripper_NilBody(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "http://localhost", http.NoBody)
	require.NoError(t, err)
	req.Body = nil
	req.GetBody = nil

	recorder := &recordingRoundTripper{}
	rt := &getBodyFixRoundTripper{base: recorder}

	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Nil(t, recorder.req.GetBody)
}

type unexpectedEOFReader struct{}

func (*unexpectedEOFReader) Read([]byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}

func TestGetBodyFixRoundTripper_ReadAllError(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "http://localhost", http.NoBody)
	require.NoError(t, err)
	req.Body = io.NopCloser(&unexpectedEOFReader{})
	req.GetBody = nil

	recorder := &recordingRoundTripper{}
	rt := &getBodyFixRoundTripper{base: recorder}

	resp, err := rt.RoundTrip(req)
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	assert.Nil(t, resp)
	assert.Nil(t, recorder.req, "base RoundTripper should not have been called")
}

func TestGetHTTPRoundTripperCustomHeaders(t *testing.T) {
	var (
		mu                 sync.Mutex
		gotHeader, gotHost string
	)
	testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		mu.Lock()
		gotHeader = req.Header.Get("X-Custom-Header")
		gotHost = req.Host
		mu.Unlock()
		res.WriteHeader(http.StatusOK)
	}))
	defer testServer.Close()

	cfg := &config.Configuration{
		TLS: configtls.ClientConfig{Insecure: true},
		CustomHeaders: map[string]string{
			"X-Custom-Header": "custom-value",
			"Host":            "signed.example.com",
		},
	}
	rt, err := GetHTTPRoundTripper(context.Background(), cfg, zap.NewNop(), nil)
	require.NoError(t, err)

	client := &http.Client{Transport: rt}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, testServer.URL, http.NoBody)
	require.NoError(t, err)
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, "custom-value", gotHeader)
	assert.Equal(t, "signed.example.com", gotHost)
}

func TestGetHTTPRoundTripperCustomHeadersDoNotMutateOriginalRequest(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, _ *http.Request) {
		res.WriteHeader(http.StatusOK)
	}))
	defer testServer.Close()

	cfg := &config.Configuration{
		TLS:           configtls.ClientConfig{Insecure: true},
		CustomHeaders: map[string]string{"X-Custom-Header": "custom-value"},
	}
	rt, err := GetHTTPRoundTripper(context.Background(), cfg, zap.NewNop(), nil)
	require.NoError(t, err)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, testServer.URL, http.NoBody)
	require.NoError(t, err)
	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Empty(t, req.Header.Get("X-Custom-Header"))
}

func TestGetHTTPRoundTripperCustomHeadersDoNotOverridePerRequestHeaders(t *testing.T) {
	var (
		mu        sync.Mutex
		gotHeader string
	)
	testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		mu.Lock()
		gotHeader = req.Header.Get("X-Custom-Header")
		mu.Unlock()
		res.WriteHeader(http.StatusOK)
	}))
	defer testServer.Close()

	cfg := &config.Configuration{
		TLS:           configtls.ClientConfig{Insecure: true},
		CustomHeaders: map[string]string{"X-Custom-Header": "static-value"},
	}
	rt, err := GetHTTPRoundTripper(context.Background(), cfg, zap.NewNop(), nil)
	require.NoError(t, err)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, testServer.URL, http.NoBody)
	require.NoError(t, err)
	req.Header.Set("X-Custom-Header", "per-request-value")
	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, "per-request-value", gotHeader)
}
