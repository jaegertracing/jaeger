// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configtls"

	"github.com/jaegertracing/jaeger/cmd/collector/app/flags"
	"github.com/jaegertracing/jaeger/internal/tenancy"
	"github.com/jaegertracing/jaeger/internal/testutils"
	"github.com/jaegertracing/jaeger/ports"
)

func TestSpanCollectorZipkinTLS(t *testing.T) {
	const testCertKeyLocation = "../../../../pkg/config/tlscfg/testdata"
	testCases := []struct {
		name                  string
		serverTLS             configtls.ServerConfig
		clientTLS             configtls.ClientConfig
		expectTLSClientErr    bool
		expectZipkinClientErr bool
		expectServerFail      bool
	}{
		{
			name: "should fail with TLS client to untrusted TLS server",
			serverTLS: configtls.ServerConfig{
				Config: configtls.Config{
					CertFile: testCertKeyLocation + "/example-server-cert.pem",
					KeyFile:  testCertKeyLocation + "/example-server-key.pem",
				},
			},
			clientTLS: configtls.ClientConfig{
				ServerName: "example.com",
			},
			expectTLSClientErr:    true,
			expectZipkinClientErr: true,
			expectServerFail:      false,
		},
		{
			name: "should fail with TLS client to trusted TLS server with incorrect hostname",
			serverTLS: configtls.ServerConfig{
				Config: configtls.Config{
					CertFile: testCertKeyLocation + "/example-server-cert.pem",
					KeyFile:  testCertKeyLocation + "/example-server-key.pem",
				},
			},
			clientTLS: configtls.ClientConfig{
				Config: configtls.Config{
					CAFile: testCertKeyLocation + "/example-CA-cert.pem",
				},
				ServerName: "nonEmpty",
			},
			expectTLSClientErr:    true,
			expectZipkinClientErr: true,
			expectServerFail:      false,
		},
		{
			name: "should pass with TLS client to trusted TLS server with correct hostname",
			serverTLS: configtls.ServerConfig{
				Config: configtls.Config{
					CertFile: testCertKeyLocation + "/example-server-cert.pem",
					KeyFile:  testCertKeyLocation + "/example-server-key.pem",
				},
			},
			clientTLS: configtls.ClientConfig{
				Config: configtls.Config{
					CAFile: testCertKeyLocation + "/example-CA-cert.pem",
				},
				ServerName: "example.com",
			},
			expectTLSClientErr:    false,
			expectZipkinClientErr: false,
			expectServerFail:      false,
		},
		{
			name: "should fail with TLS client without cert to trusted TLS server requiring cert",
			serverTLS: configtls.ServerConfig{
				ClientCAFile: testCertKeyLocation + "/example-CA-cert.pem",
				Config: configtls.Config{
					CertFile: testCertKeyLocation + "/example-server-cert.pem",
					KeyFile:  testCertKeyLocation + "/example-server-key.pem",
				},
			},
			clientTLS: configtls.ClientConfig{
				Config: configtls.Config{
					CAFile: testCertKeyLocation + "/example-CA-cert.pem",
				},
				ServerName: "example.com",
			},
			expectTLSClientErr:    false,
			expectZipkinClientErr: true,
			expectServerFail:      false,
		},
		{
			name: "should pass with TLS client with cert to trusted TLS server requiring cert",
			serverTLS: configtls.ServerConfig{
				ClientCAFile: testCertKeyLocation + "/example-CA-cert.pem",
				Config: configtls.Config{
					CertFile: testCertKeyLocation + "/example-server-cert.pem",
					KeyFile:  testCertKeyLocation + "/example-server-key.pem",
				},
			},
			clientTLS: configtls.ClientConfig{
				Config: configtls.Config{
					CAFile:   testCertKeyLocation + "/example-CA-cert.pem",
					CertFile: testCertKeyLocation + "/example-client-cert.pem",
					KeyFile:  testCertKeyLocation + "/example-client-key.pem",
				},
				ServerName: "example.com",
			},
			expectTLSClientErr:    false,
			expectZipkinClientErr: false,
			expectServerFail:      false,
		},
		{
			name: "should fail with TLS client without cert to trusted TLS server requiring cert from different CA",
			serverTLS: configtls.ServerConfig{
				ClientCAFile: testCertKeyLocation + "/wrong-CA-cert.pem",
				Config: configtls.Config{
					CertFile: testCertKeyLocation + "/example-server-cert.pem",
					KeyFile:  testCertKeyLocation + "/example-server-key.pem",
				},
			},
			clientTLS: configtls.ClientConfig{
				Config: configtls.Config{
					CAFile:   testCertKeyLocation + "/example-CA-cert.pem",
					CertFile: testCertKeyLocation + "/example-client-cert.pem",
					KeyFile:  testCertKeyLocation + "/example-client-key.pem",
				},
				ServerName: "example.com",
			},
			expectTLSClientErr:    false,
			expectZipkinClientErr: true,
			expectServerFail:      false,
		},
		{
			name: "should fail with TLS client with cert to trusted TLS server with incorrect TLS min",
			serverTLS: configtls.ServerConfig{
				ClientCAFile: testCertKeyLocation + "/example-CA-cert.pem",
				Config: configtls.Config{
					CertFile:   testCertKeyLocation + "/example-server-cert.pem",
					KeyFile:    testCertKeyLocation + "/example-server-key.pem",
					MinVersion: "1.5",
				},
			},
			clientTLS: configtls.ClientConfig{
				Config: configtls.Config{
					CAFile:   testCertKeyLocation + "/example-CA-cert.pem",
					CertFile: testCertKeyLocation + "/example-client-cert.pem",
					KeyFile:  testCertKeyLocation + "/example-client-key.pem",
				},
				ServerName: "example.com",
			},
			expectTLSClientErr:    true,
			expectServerFail:      true,
			expectZipkinClientErr: false,
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			spanProcessor := &mockSpanProcessor{}
			logger, _ := testutils.NewLogger()
			tm := &tenancy.Manager{}

			opts := &flags.CollectorOptions{}
			opts.Zipkin.Endpoint = ports.PortToHostPort(ports.CollectorZipkin)
			opts.Zipkin.TLSSetting = &test.serverTLS

			server, err := StartZipkinReceiver(opts, logger, spanProcessor, tm)
			if test.expectServerFail {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			defer func() {
				require.NoError(t, server.Shutdown(context.Background()))
			}()

			clientTLSCfg, err0 := test.clientTLS.LoadTLSConfig(context.Background())
			require.NoError(t, err0)
			dialer := &net.Dialer{Timeout: 2 * time.Second}
			conn, clientError := tls.DialWithDialer(dialer, "tcp", fmt.Sprintf("localhost:%d", ports.CollectorZipkin), clientTLSCfg)

			if test.expectTLSClientErr {
				require.Error(t, clientError)
			} else {
				require.NoError(t, clientError)
				require.NoError(t, conn.Close())
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
				require.NoError(t, response.Body.Close())
			}
		})
	}
}
