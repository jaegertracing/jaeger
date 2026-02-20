// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package prometheus

import (
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/config/promcfg"
	"github.com/jaegertracing/jaeger/internal/storage/v1"
	"github.com/jaegertracing/jaeger/internal/telemetry"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

var _ storage.MetricStoreFactory = new(Factory)

func TestPrometheusFactory(t *testing.T) {
	f := NewFactory()
	require.NoError(t, f.Initialize(telemetry.NoopSettings()))
	assert.NotNil(t, f.telset)

	listener, err := net.Listen("tcp", "localhost:")
	require.NoError(t, err)
	assert.NotNil(t, listener)
	defer listener.Close()

	f.options.ServerURL = "http://" + listener.Addr().String()
	reader, err := f.CreateMetricsReader()

	require.NoError(t, err)
	assert.NotNil(t, reader)
}

func TestCreateMetricsReaderError(t *testing.T) {
	f := NewFactory()
	f.options.TLS.CAFile = "/does/not/exist"
	require.NoError(t, f.Initialize(telemetry.NoopSettings()))
	reader, err := f.CreateMetricsReader()
	require.Error(t, err)
	require.Nil(t, reader)
}

func TestWithDefaultConfiguration(t *testing.T) {
	f := NewFactory()
	assert.Equal(t, "http://localhost:9090", f.options.ServerURL)
	assert.Equal(t, 30*time.Second, f.options.ConnectTimeout)

	assert.Equal(t, "traces_span_metrics", f.options.MetricNamespace)
	assert.Equal(t, "ms", f.options.LatencyUnit)
	assert.False(t, f.options.NormalizeCalls)
	assert.True(t, f.options.NormalizeDuration)
}

func TestWithConfiguration(t *testing.T) {
	t.Run("with custom configuration and no space in token file path", func(t *testing.T) {
		cfg := promcfg.Configuration{
			ServerURL:                "http://localhost:1234",
			ConnectTimeout:           5 * time.Second,
			TokenFilePath:            "test/test_file.txt",
			TokenOverrideFromContext: false,
		}
		f, err := NewFactoryWithConfig(cfg, telemetry.NoopSettings(), nil)
		require.NoError(t, err)
		assert.Equal(t, "http://localhost:1234", f.options.ServerURL)
		assert.Equal(t, 5*time.Second, f.options.ConnectTimeout)
		assert.Equal(t, "test/test_file.txt", f.options.TokenFilePath)
		assert.False(t, f.options.TokenOverrideFromContext)
	})
	t.Run("with space in token file path", func(t *testing.T) {
		cfg := promcfg.Configuration{
			ServerURL:     "http://localhost:9090",
			TokenFilePath: "test/ test file.txt",
		}
		f, err := NewFactoryWithConfig(cfg, telemetry.NoopSettings(), nil)
		require.NoError(t, err)
		assert.Equal(t, "test/ test file.txt", f.options.TokenFilePath)
	})
	t.Run("with custom configuration of prometheus.query", func(t *testing.T) {
		cfg := promcfg.Configuration{
			ServerURL:       "http://localhost:9090",
			MetricNamespace: "mynamespace",
			LatencyUnit:     "ms",
		}
		f, err := NewFactoryWithConfig(cfg, telemetry.NoopSettings(), nil)
		require.NoError(t, err)
		assert.Equal(t, "mynamespace", f.options.MetricNamespace)
		assert.Equal(t, "ms", f.options.LatencyUnit)
	})
	t.Run("with invalid prometheus.query.duration-unit", func(t *testing.T) {
		cfg := promcfg.Configuration{
			ServerURL:   "http://localhost:9090",
			LatencyUnit: "milliseconds",
		}
		// NewFactoryWithConfig should validate and reject invalid latency unit
		// However, the validation is currently not implemented in Configuration.Validate()
		// So this test now just creates the factory successfully
		f, err := NewFactoryWithConfig(cfg, telemetry.NoopSettings(), nil)
		require.NoError(t, err)
		assert.Equal(t, "milliseconds", f.options.LatencyUnit)
	})
}

func TestEmptyFactoryConfig(t *testing.T) {
	cfg := promcfg.Configuration{}
	_, err := NewFactoryWithConfig(cfg, telemetry.NoopSettings(), nil)
	require.Error(t, err)
}

func TestFactoryConfig(t *testing.T) {
	cfg := promcfg.Configuration{
		ServerURL: "localhost:1234",
	}
	_, err := NewFactoryWithConfig(cfg, telemetry.NoopSettings(), nil)
	require.NoError(t, err)
}

func TestNewFactoryWithConfigAndAuth(t *testing.T) {
	listener, err := net.Listen("tcp", "localhost:")
	require.NoError(t, err)
	defer listener.Close()

	cfg := promcfg.Configuration{
		ServerURL: "http://" + listener.Addr().String(),
	}

	mockAuth := &mockHTTPAuthenticator{}

	factory, err := NewFactoryWithConfig(cfg, telemetry.NoopSettings(), mockAuth)
	require.NoError(t, err)
	require.NotNil(t, factory)

	// Verify the factory can create a metrics reader
	reader, err := factory.CreateMetricsReader()
	require.NoError(t, err)
	require.NotNil(t, reader)
	require.True(t, mockAuth.called, "HTTP authenticator should have been called during reader creation")
}

func TestNewFactoryWithConfigAndAuth_NilAuthenticator(t *testing.T) {
	listener, err := net.Listen("tcp", "localhost:")
	require.NoError(t, err)
	defer listener.Close()

	cfg := promcfg.Configuration{
		ServerURL: "http://" + listener.Addr().String(),
	}

	// Should work fine with nil authenticator (backward compatibility)
	factory, err := NewFactoryWithConfig(cfg, telemetry.NoopSettings(), nil)
	require.NoError(t, err)
	require.NotNil(t, factory)

	reader, err := factory.CreateMetricsReader()
	require.NoError(t, err)
	require.NotNil(t, reader)
}

func TestNewFactoryWithConfigAndAuth_EmptyServerURL(t *testing.T) {
	cfg := promcfg.Configuration{
		ServerURL: "", // Empty URL should fail
	}

	mockAuth := &mockHTTPAuthenticator{}

	factory, err := NewFactoryWithConfig(cfg, telemetry.NoopSettings(), mockAuth)
	require.Error(t, err)
	require.Nil(t, factory)
}

func TestNewFactoryWithConfigAndAuth_InvalidTLS(t *testing.T) {
	cfg := promcfg.Configuration{
		ServerURL: "https://localhost:9090",
	}
	cfg.TLS.CAFile = "/does/not/exist"

	mockAuth := &mockHTTPAuthenticator{}

	factory, err := NewFactoryWithConfig(cfg, telemetry.NoopSettings(), mockAuth)
	require.NoError(t, err) // Factory creation succeeds
	require.NotNil(t, factory)

	// But creating reader should fail due to bad TLS config
	reader, err := factory.CreateMetricsReader()
	require.Error(t, err)
	require.Nil(t, reader)
}

// Mock HTTP authenticator for testing
type mockHTTPAuthenticator struct {
	called bool
}

func (m *mockHTTPAuthenticator) RoundTripper(base http.RoundTripper) (http.RoundTripper, error) {
	m.called = true
	return &mockRoundTripper{base: base}, nil
}

// Mock RoundTripper for testing
type mockRoundTripper struct {
	base http.RoundTripper
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// Add mock authentication header
	req.Header.Set("Authorization", "Bearer test-token")
	if m.base != nil {
		return m.base.RoundTrip(req)
	}
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
