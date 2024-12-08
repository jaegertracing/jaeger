// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/config/configtls"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/collector/app/handler"
	"github.com/jaegertracing/jaeger/internal/metricstest"
	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
	"github.com/jaegertracing/jaeger/pkg/healthcheck"
	"github.com/jaegertracing/jaeger/ports"
)

var testCertKeyLocation = "../../../../pkg/config/tlscfg/testdata"

// test wrong port number
func TestFailToListenHTTP(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	server, err := StartHTTPServer(&HTTPServerParams{
		ServerConfig: confighttp.ServerConfig{
			Endpoint: ":-1",
		},
		Logger: logger,
	})
	assert.Nil(t, server)
	require.EqualError(t, err, "listen tcp: address -1: invalid port")
}

func TestCreateTLSHTTPServerError(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	params := &HTTPServerParams{
		ServerConfig: confighttp.ServerConfig{
			Endpoint: ":0",
			TLSSetting: &configtls.ServerConfig{
				Config: configtls.Config{
					CertFile: "invalid/path",
					KeyFile:  "invalid/path",
					CAFile:   "invalid/path",
				},
			},
		},
		HealthCheck: healthcheck.New(),
		Logger:      logger,
	}
	_, err := StartHTTPServer(params)
	require.Error(t, err)
}

func TestSpanCollectorHTTP(t *testing.T) {
	mFact := metricstest.NewFactory(time.Hour)
	defer mFact.Backend.Stop()
	logger, _ := zap.NewDevelopment()
	params := &HTTPServerParams{
		Handler:          handler.NewJaegerSpanHandler(logger, &mockSpanProcessor{}),
		SamplingProvider: &mockSamplingProvider{},
		MetricsFactory:   mFact,
		HealthCheck:      healthcheck.New(),
		Logger:           logger,
	}

	server, _ := params.ToServer(context.Background(), nil, component.TelemetrySettings{}, nil)
	listener, _ := params.ToListener(context.Background())

	serveHTTP(server, listener, params)
	addr := listener.Addr().String()
	host, port, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	url := fmt.Sprintf("http://%s:%s", host, port)
	response, err := http.Post(url, "", nil)
	require.NoError(t, err)
	assert.NotNil(t, response)
	defer response.Body.Close()
	defer server.Close()
}

func TestSpanCollectorHTTPS(t *testing.T) {
	testCases := []struct {
		name              string
		TLS               tlscfg.Options
		clientTLS         tlscfg.Options
		expectError       bool
		expectClientError bool
	}{
		{
			name: "should fail with TLS client to untrusted TLS server",
			TLS: tlscfg.Options{
				Enabled:  true,
				CertPath: testCertKeyLocation + "/example-server-cert.pem",
				KeyPath:  testCertKeyLocation + "/example-server-key.pem",
			},
			clientTLS: tlscfg.Options{
				Enabled:    true,
				ServerName: "example.com",
			},
			expectError:       true,
			expectClientError: true,
		},
		{
			name: "should fail with TLS client to trusted TLS server with incorrect hostname",
			TLS: tlscfg.Options{
				Enabled:  true,
				CertPath: testCertKeyLocation + "/example-server-cert.pem",
				KeyPath:  testCertKeyLocation + "/example-server-key.pem",
			},
			clientTLS: tlscfg.Options{
				Enabled:    true,
				CAPath:     testCertKeyLocation + "/example-CA-cert.pem",
				ServerName: "nonEmpty",
			},
			expectError:       true,
			expectClientError: true,
		},
		{
			name: "should pass with TLS client to trusted TLS server with correct hostname",
			TLS: tlscfg.Options{
				Enabled:  true,
				CertPath: testCertKeyLocation + "/example-server-cert.pem",
				KeyPath:  testCertKeyLocation + "/example-server-key.pem",
			},
			clientTLS: tlscfg.Options{
				Enabled:    true,
				CAPath:     testCertKeyLocation + "/example-CA-cert.pem",
				ServerName: "example.com",
			},
		},
		{
			name: "should fail with TLS client without cert to trusted TLS server requiring cert",
			TLS: tlscfg.Options{
				Enabled:      true,
				CertPath:     testCertKeyLocation + "/example-server-cert.pem",
				KeyPath:      testCertKeyLocation + "/example-server-key.pem",
				ClientCAPath: testCertKeyLocation + "/example-CA-cert.pem",
			},
			clientTLS: tlscfg.Options{
				Enabled:    true,
				CAPath:     testCertKeyLocation + "/example-CA-cert.pem",
				ServerName: "example.com",
			},
			expectClientError: true,
		},
		{
			name: "should pass with TLS client with cert to trusted TLS server requiring cert",
			TLS: tlscfg.Options{
				Enabled:      true,
				CertPath:     testCertKeyLocation + "/example-server-cert.pem",
				KeyPath:      testCertKeyLocation + "/example-server-key.pem",
				ClientCAPath: testCertKeyLocation + "/example-CA-cert.pem",
			},
			clientTLS: tlscfg.Options{
				Enabled:    true,
				CAPath:     testCertKeyLocation + "/example-CA-cert.pem",
				ServerName: "example.com",
				CertPath:   testCertKeyLocation + "/example-client-cert.pem",
				KeyPath:    testCertKeyLocation + "/example-client-key.pem",
			},
		},
		{
			name: "should fail with TLS client without cert to trusted TLS server requiring cert from a different CA",
			TLS: tlscfg.Options{
				Enabled:      true,
				CertPath:     testCertKeyLocation + "/example-server-cert.pem",
				KeyPath:      testCertKeyLocation + "/example-server-key.pem",
				ClientCAPath: testCertKeyLocation + "/wrong-CA-cert.pem", // NB: wrong CA
			},
			clientTLS: tlscfg.Options{
				Enabled:    true,
				CAPath:     testCertKeyLocation + "/example-CA-cert.pem",
				ServerName: "example.com",
				CertPath:   testCertKeyLocation + "/example-client-cert.pem",
				KeyPath:    testCertKeyLocation + "/example-client-key.pem",
			},
			expectClientError: true,
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			// Cannot reliably use zaptest.NewLogger(t) because it causes race condition
			// See https://github.com/jaegertracing/jaeger/issues/4497.
			logger := zap.NewNop()
			mFact := metricstest.NewFactory(time.Hour)
			defer mFact.Backend.Stop()
			params := &HTTPServerParams{
				ServerConfig: confighttp.ServerConfig{
					Endpoint:   fmt.Sprintf(":%d", ports.CollectorHTTP),
					TLSSetting: test.TLS.ToOtelServerConfig(),
				},
				Handler:          handler.NewJaegerSpanHandler(logger, &mockSpanProcessor{}),
				SamplingProvider: &mockSamplingProvider{},
				MetricsFactory:   mFact,
				HealthCheck:      healthcheck.New(),
				Logger:           logger,
			}

			server, err := StartHTTPServer(params)
			require.NoError(t, err)
			defer func() {
				require.NoError(t, server.Close())
			}()

			clientTLSCfg, err0 := test.clientTLS.ToOtelClientConfig().LoadTLSConfig(context.Background())
			require.NoError(t, err0)
			dialer := &net.Dialer{Timeout: 2 * time.Second}
			conn, clientError := tls.DialWithDialer(dialer, "tcp", "localhost:"+strconv.Itoa(ports.CollectorHTTP), clientTLSCfg)
			var clientClose func() error
			clientClose = nil
			if conn != nil {
				clientClose = conn.Close
			}

			if test.expectError {
				require.Error(t, clientError)
			} else {
				require.NoError(t, clientError)
			}

			if clientClose != nil {
				require.NoError(t, clientClose())
			}

			client := &http.Client{
				Transport: &http.Transport{
					TLSClientConfig: clientTLSCfg,
				},
			}

			response, requestError := client.Post("https://localhost:"+strconv.Itoa(ports.CollectorHTTP), "", nil)

			if test.expectClientError {
				require.Error(t, requestError)
			} else {
				require.NoError(t, requestError)
				require.NotNil(t, response)
				// ensures that the body has been initialized attempting to close
				defer response.Body.Close()
			}
		})
	}
}

func TestStartHTTPServerParams(t *testing.T) {
	logger := zap.NewNop()
	mFact := metricstest.NewFactory(time.Hour)
	defer mFact.Stop()
	params := &HTTPServerParams{
		ServerConfig: confighttp.ServerConfig{
			Endpoint:          fmt.Sprintf(":%d", ports.CollectorHTTP),
			IdleTimeout:       5 * time.Minute,
			ReadTimeout:       6 * time.Minute,
			ReadHeaderTimeout: 7 * time.Second,
		},
		Handler:          handler.NewJaegerSpanHandler(logger, &mockSpanProcessor{}),
		SamplingProvider: &mockSamplingProvider{},
		MetricsFactory:   mFact,
		HealthCheck:      healthcheck.New(),
		Logger:           logger,
	}

	server, err := StartHTTPServer(params)
	require.NoError(t, err)
	defer server.Close()
	assert.Equal(t, 5*time.Minute, server.IdleTimeout)
	assert.Equal(t, 6*time.Minute, server.ReadTimeout)
	assert.Equal(t, 7*time.Second, server.ReadHeaderTimeout)
}
