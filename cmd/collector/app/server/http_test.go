// Copyright (c) 2020 The Jaeger Authors.
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

package server

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		HostPort: ":-1",
		Logger:   logger,
	})
	assert.Nil(t, server)
	require.EqualError(t, err, "listen tcp: address -1: invalid port")
}

func TestCreateTLSHTTPServerError(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	tlsCfg := tlscfg.Options{
		Enabled:      true,
		CertPath:     "invalid/path",
		KeyPath:      "invalid/path",
		ClientCAPath: "invalid/path",
	}

	params := &HTTPServerParams{
		HostPort:    fmt.Sprintf(":%d", ports.CollectorHTTP),
		HealthCheck: healthcheck.New(),
		Logger:      logger,
		TLSConfig:   tlsCfg,
	}
	_, err := StartHTTPServer(params)
	require.Error(t, err)
	params.TLSConfig.Close()
}

func TestSpanCollectorHTTP(t *testing.T) {
	mFact := metricstest.NewFactory(time.Hour)
	defer mFact.Backend.Stop()
	logger, _ := zap.NewDevelopment()
	params := &HTTPServerParams{
		Handler:        handler.NewJaegerSpanHandler(logger, &mockSpanProcessor{}),
		SamplingStore:  &mockSamplingStore{},
		MetricsFactory: mFact,
		HealthCheck:    healthcheck.New(),
		Logger:         logger,
	}

	server := httptest.NewServer(nil)

	serveHTTP(server.Config, server.Listener, params)

	response, err := http.Post(server.URL, "", nil)
	require.NoError(t, err)
	assert.NotNil(t, response)
	response.Body.Close()
	server.Close()
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
				HostPort:       fmt.Sprintf(":%d", ports.CollectorHTTP),
				Handler:        handler.NewJaegerSpanHandler(logger, &mockSpanProcessor{}),
				SamplingStore:  &mockSamplingStore{},
				MetricsFactory: mFact,
				HealthCheck:    healthcheck.New(),
				Logger:         logger,
				TLSConfig:      test.TLS,
			}
			defer params.TLSConfig.Close()

			server, err := StartHTTPServer(params)
			require.NoError(t, err)
			defer func() {
				require.NoError(t, server.Close())
			}()

			clientTLSCfg, err0 := test.clientTLS.Config(logger)
			defer test.clientTLS.Close()
			require.NoError(t, err0)
			dialer := &net.Dialer{Timeout: 2 * time.Second}
			conn, clientError := tls.DialWithDialer(dialer, "tcp", "localhost:"+fmt.Sprintf("%d", ports.CollectorHTTP), clientTLSCfg)
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

			response, requestError := client.Post("https://localhost:"+fmt.Sprintf("%d", ports.CollectorHTTP), "", nil)

			if test.expectClientError {
				require.Error(t, requestError)
			} else {
				require.NoError(t, requestError)
				require.NotNil(t, response)
				response.Body.Close()
			}
		})
	}
}

func TestStartHTTPServerParams(t *testing.T) {
	logger := zap.NewNop()
	mFact := metricstest.NewFactory(time.Hour)
	defer mFact.Stop()
	params := &HTTPServerParams{
		HostPort:          fmt.Sprintf(":%d", ports.CollectorHTTP),
		Handler:           handler.NewJaegerSpanHandler(logger, &mockSpanProcessor{}),
		SamplingStore:     &mockSamplingStore{},
		MetricsFactory:    mFact,
		HealthCheck:       healthcheck.New(),
		Logger:            logger,
		IdleTimeout:       5 * time.Minute,
		ReadTimeout:       6 * time.Minute,
		ReadHeaderTimeout: 7 * time.Second,
	}

	server, err := StartHTTPServer(params)
	require.NoError(t, err)
	defer server.Close()
	assert.Equal(t, 5*time.Minute, server.IdleTimeout)
	assert.Equal(t, 6*time.Minute, server.ReadTimeout)
	assert.Equal(t, 7*time.Second, server.ReadHeaderTimeout)
}
