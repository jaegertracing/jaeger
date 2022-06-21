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

// test wrong port number
func TestFailToListenZipkin(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	server, err := StartZipkinServer(&ZipkinServerParams{
		HostPort: ":-1",
		Logger:   logger,
	})
	assert.Nil(t, server)
	assert.EqualError(t, err, "listen tcp: address -1: invalid port")
}

func TestSpanCollectorZipkin(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	params := &ZipkinServerParams{
		Handler:        handler.NewZipkinSpanHandler(logger, nil, nil),
		MetricsFactory: metricstest.NewFactory(time.Hour),
		HealthCheck:    healthcheck.New(),
		Logger:         logger,
	}

	server := httptest.NewServer(nil)
	defer server.Close()

	serveZipkin(server.Config, server.Listener, params)

	response, err := http.Post(server.URL, "", nil)
	assert.NoError(t, err)
	assert.NotNil(t, response)
}

func TestSpanCollectorZipkinTLS(t *testing.T) {
	testCases := []struct {
		name                  string
		serverTLS             tlscfg.Options
		clientTLS             tlscfg.Options
		expectTLSClientErr    bool
		expectZipkinClientErr bool
		expectServerFail      bool
	}{
		{
			name: "should fail with TLS client to untrusted TLS server",
			serverTLS: tlscfg.Options{
				Enabled:  true,
				CertPath: testCertKeyLocation + "/example-server-cert.pem",
				KeyPath:  testCertKeyLocation + "/example-server-key.pem",
			},
			clientTLS: tlscfg.Options{
				Enabled:    true,
				ServerName: "example.com",
			},
			expectTLSClientErr:    true,
			expectZipkinClientErr: true,
			expectServerFail:      false,
		},
		{
			name: "should fail with TLS client to trusted TLS server with incorrect hostname",
			serverTLS: tlscfg.Options{
				Enabled:  true,
				CertPath: testCertKeyLocation + "/example-server-cert.pem",
				KeyPath:  testCertKeyLocation + "/example-server-key.pem",
			},
			clientTLS: tlscfg.Options{
				Enabled:    true,
				CAPath:     testCertKeyLocation + "/example-CA-cert.pem",
				ServerName: "nonEmpty",
			},
			expectTLSClientErr:    true,
			expectZipkinClientErr: true,
			expectServerFail:      false,
		},
		{
			name: "should pass with TLS client to trusted TLS server with correct hostname",
			serverTLS: tlscfg.Options{
				Enabled:  true,
				CertPath: testCertKeyLocation + "/example-server-cert.pem",
				KeyPath:  testCertKeyLocation + "/example-server-key.pem",
			},
			clientTLS: tlscfg.Options{
				Enabled:    true,
				CAPath:     testCertKeyLocation + "/example-CA-cert.pem",
				ServerName: "example.com",
			},
			expectTLSClientErr:    false,
			expectZipkinClientErr: false,
			expectServerFail:      false,
		},
		{
			name: "should fail with TLS client without cert to trusted TLS server requiring cert",
			serverTLS: tlscfg.Options{
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
			expectTLSClientErr:    false,
			expectServerFail:      false,
			expectZipkinClientErr: true,
		},
		{
			name: "should pass with TLS client with cert to trusted TLS server requiring cert",
			serverTLS: tlscfg.Options{
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
			expectTLSClientErr:    false,
			expectServerFail:      false,
			expectZipkinClientErr: false,
		},
		{
			name: "should fail with TLS client without cert to trusted TLS server requiring cert from a different CA",
			serverTLS: tlscfg.Options{
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
			expectTLSClientErr:    false,
			expectServerFail:      false,
			expectZipkinClientErr: true,
		},
		{
			name: "should fail with TLS client with cert to trusted TLS server with incorrect TLS min",
			serverTLS: tlscfg.Options{
				Enabled:      true,
				CertPath:     testCertKeyLocation + "/example-server-cert.pem",
				KeyPath:      testCertKeyLocation + "/example-server-key.pem",
				ClientCAPath: testCertKeyLocation + "/example-CA-cert.pem",
				MinVersion:   "1.5",
			},
			clientTLS: tlscfg.Options{
				Enabled:    true,
				CAPath:     testCertKeyLocation + "/example-CA-cert.pem",
				ServerName: "example.com",
				CertPath:   testCertKeyLocation + "/example-client-cert.pem",
				KeyPath:    testCertKeyLocation + "/example-client-key.pem",
			},
			expectTLSClientErr:    true,
			expectServerFail:      true,
			expectZipkinClientErr: false,
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			logger, _ := zap.NewDevelopment()
			params := &ZipkinServerParams{
				HostPort:       fmt.Sprintf(":%d", ports.CollectorZipkin),
				Handler:        handler.NewZipkinSpanHandler(logger, nil, nil),
				MetricsFactory: metricstest.NewFactory(time.Hour),
				HealthCheck:    healthcheck.New(),
				Logger:         logger,
				TLSConfig:      test.serverTLS,
			}

			server, err := StartZipkinServer(params)

			if test.expectServerFail {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			defer server.Close()

			clientTLSCfg, err0 := test.clientTLS.Config(zap.NewNop())
			require.NoError(t, err0)
			dialer := &net.Dialer{Timeout: 2 * time.Second}
			conn, clientError := tls.DialWithDialer(dialer, "tcp", fmt.Sprintf("localhost:%d", ports.CollectorZipkin), clientTLSCfg)

			if test.expectTLSClientErr {
				require.Error(t, clientError)
			} else {
				require.NoError(t, clientError)
				require.Nil(t, conn.Close())
			}

			client := &http.Client{
				Transport: &http.Transport{
					TLSClientConfig: clientTLSCfg,
				},
			}

			response, requestError := client.Post(fmt.Sprintf("https://localhost:%d", ports.CollectorZipkin), "", nil)

			if test.expectZipkinClientErr {
				require.Error(t, requestError)
			} else {
				require.NoError(t, requestError)
				require.NotNil(t, response)
			}
		})
	}
}
