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
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/collector/app/flags"
	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/ports"
)

func TestSpanCollectorZipkinTLS(t *testing.T) {
	const testCertKeyLocation = "../../../../pkg/config/tlscfg/testdata"
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
			spanProcessor := &mockSpanProcessor{}
			logger, _ := testutils.NewLogger()
			tm := &tenancy.Manager{}

			opts := &flags.CollectorOptions{}
			opts.Zipkin.HTTPHostPort = ports.PortToHostPort(ports.CollectorZipkin)
			opts.Zipkin.TLS = test.serverTLS
			defer test.serverTLS.Close()

			server, err := StartZipkinReceiver(opts, logger, spanProcessor, tm)
			if test.expectServerFail {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			defer func() {
				require.NoError(t, server.Shutdown(context.Background()))
			}()

			clientTLSCfg, err0 := test.clientTLS.Config(zap.NewNop())
			defer test.clientTLS.Close()
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
