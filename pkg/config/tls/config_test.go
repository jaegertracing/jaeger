// Copyright (c) 2019 The Jaeger Authors.
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
package tls

import (
	"crypto/x509"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProxyBuilder(t *testing.T) {
	tests := []struct {
		name        string
		options     Options
		fakeSysPool bool
		expectError string
	}{
		{
			name:    "should load system CA",
			options: Options{CAPath: ""},
		},
		{
			name:        "should fail with fake system CA",
			fakeSysPool: true,
			options:     Options{CAPath: ""},
			expectError: "fake system pool",
		},
		{
			name:    "should load custom CA",
			options: Options{CAPath: "testdata/testCA.pem"},
		},
		{
			name:        "should fail with invalid CA file path",
			options:     Options{CAPath: "testdata/not/valid"},
			expectError: "failed to read client CAs",
		},
		{
			name:        "should fail with invalid CA file content",
			options:     Options{CAPath: "testdata/testCA-bad.txt"},
			expectError: "failed to build client CAs",
		},
		{
			name: "should load valid TLS Client settings",
			options: Options{
				CAPath:   "testdata/testCA.pem",
				CertPath: "testdata/test-cert.pem",
				KeyPath:  "testdata/test-key.pem",
			},
		},
		{
			name: "should fail with missing TLS Client Key",
			options: Options{
				CAPath:   "testdata/testCA.pem",
				CertPath: "testdata/test-cert.pem",
			},
			expectError: "both client certificate and key must be supplied",
		},
		{
			name: "should fail with invalid TLS Client Key",
			options: Options{
				CAPath:   "testdata/testCA.pem",
				CertPath: "testdata/test-cert.pem",
				KeyPath:  "testdata/not/valid",
			},
			expectError: "failed to load server TLS cert and key",
		},
		{
			name: "should fail with missing TLS Client Cert",
			options: Options{
				CAPath:  "testdata/testCA.pem",
				KeyPath: "testdata/test-key.pem",
			},
			expectError: "both client certificate and key must be supplied",
		},
		{
			name: "should fail with invalid TLS Client Cert",
			options: Options{
				CAPath:   "testdata/testCA.pem",
				CertPath: "testdata/not/valid",
				KeyPath:  "testdata/test-key.pem",
			},
			expectError: "failed to load server TLS cert and key",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.fakeSysPool {
				saveSystemCertPool := systemCertPool
				systemCertPool = func() (*x509.CertPool, error) {
					return nil, fmt.Errorf("fake system pool")
				}
				defer func() {
					systemCertPool = saveSystemCertPool
				}()
			}
			cfg, err := test.options.Load()
			if test.expectError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), test.expectError)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, cfg)
			}
		})
	}
}

// func TestProxyClientTLS(t *testing.T) {
// 	tests := []struct {
// 		name              string
// 		grpcBuilder       *ConnBuilder
// 		serverTLS         bool
// 		serverTLSCert     string
// 		serverTLSKey      string
// 		serverTLSClientCA string
// 		expectError       bool
// 	}{
// 		{
// 			name:        "insecure grpc connection",
// 			serverTLS:   false,
// 			grpcBuilder: &ConnBuilder{},
// 			expectError: false,
// 		},
// 		{
// 			name: "TLS client to non-TLS server should fail",
// 			grpcBuilder: &ConnBuilder{
// 				TLS: true,
// 			},
// 			expectError: true,
// 		},
// 		{
// 			name:          "TLS client to untrusted TLS server should fail",
// 			serverTLS:     true,
// 			serverTLSCert: "testdata/server.jaeger.io.pem",
// 			serverTLSKey:  "testdata/server.jaeger.io-key.pem",
// 			grpcBuilder: &ConnBuilder{
// 				TLS:           true,
// 				TLSServerName: "server.jaeger.io",
// 			},
// 			expectError: true,
// 		},
// 		{
// 			name:          "TLS client to trusted TLS server with incorrect hostname should fail",
// 			serverTLS:     true,
// 			serverTLSCert: "testdata/server.jaeger.io.pem",
// 			serverTLSKey:  "testdata/server.jaeger.io-key.pem",
// 			grpcBuilder: &ConnBuilder{
// 				TLS:   true,
// 				TLSCA: "testdata/rootCA.pem",
// 			},
// 			expectError: true,
// 		},
// 		{
// 			name:          "TLS client to trusted TLS server with correct hostname",
// 			serverTLS:     true,
// 			serverTLSCert: "testdata/server.jaeger.io.pem",
// 			serverTLSKey:  "testdata/server.jaeger.io-key.pem",
// 			grpcBuilder: &ConnBuilder{
// 				TLS:           true,
// 				TLSCA:         "testdata/rootCA.pem",
// 				TLSServerName: "server.jaeger.io",
// 			},
// 			expectError: false,
// 		},
// 		{
// 			name:              "TLS client without cert to trusted TLS server requiring cert should fail",
// 			serverTLS:         true,
// 			serverTLSCert:     "testdata/server.jaeger.io.pem",
// 			serverTLSKey:      "testdata/server.jaeger.io-key.pem",
// 			serverTLSClientCA: "testdata/rootCA.pem",
// 			grpcBuilder: &ConnBuilder{
// 				TLS:           true,
// 				TLSCA:         "testdata/rootCA.pem",
// 				TLSServerName: "server.jaeger.io",
// 			},
// 			expectError: true,
// 		},
// 		{
// 			name:              "TLS client without cert to trusted TLS server requiring cert from a differe CA should fail",
// 			serverTLS:         true,
// 			serverTLSCert:     "testdata/server.jaeger.io.pem",
// 			serverTLSKey:      "testdata/server.jaeger.io-key.pem",
// 			serverTLSClientCA: "testdata/testCA.pem",
// 			grpcBuilder: &ConnBuilder{
// 				TLS:           true,
// 				TLSCA:         "testdata/rootCA.pem",
// 				TLSServerName: "server.jaeger.io",
// 				TLSCert:       "testdata/client.jaeger.io-client.pem",
// 				TLSKey:        "testdata/client.jaeger.io-client-key.pem",
// 			},
// 			expectError: true,
// 		},
// 		{
// 			name:              "TLS client without cert to trusted TLS server requiring cert should fail",
// 			serverTLS:         true,
// 			serverTLSCert:     "testdata/server.jaeger.io.pem",
// 			serverTLSKey:      "testdata/server.jaeger.io-key.pem",
// 			serverTLSClientCA: "testdata/rootCA.pem",
// 			grpcBuilder: &ConnBuilder{
// 				TLS:           true,
// 				TLSCA:         "testdata/rootCA.pem",
// 				TLSServerName: "server.jaeger.io",
// 				TLSCert:       "testdata/client.jaeger.io-client.pem",
// 				TLSKey:        "testdata/client.jaeger.io-client-key.pem",
// 			},
// 			expectError: false,
// 		},
// 	}
// 	for _, test := range tests {
// 		t.Run(test.name, func(t *testing.T) {
// 			var opts []grpc.ServerOption
// 			if test.serverTLS {
// 				tlsCfg, err := grpcserver.TLSConfig(
// 					test.serverTLSCert,
// 					test.serverTLSKey,
// 					test.serverTLSClientCA)
// 				if err != nil {
// 					require.NoError(t, err)
// 				}

// 				opts = []grpc.ServerOption{grpc.Creds(credentials.NewTLS(tlsCfg))}
// 			}

// 			spanHandler := &mockSpanHandler{}
// 			s, addr := initializeGRPCTestServer(t, func(s *grpc.Server) {
// 				api_v2.RegisterCollectorServiceServer(s, spanHandler)
// 			}, opts...)
// 			defer s.Stop()

// 			mFactory := metricstest.NewFactory(time.Microsecond)
// 			_, port, _ := net.SplitHostPort(addr.String())

// 			test.grpcBuilder.CollectorHostPorts = []string{net.JoinHostPort("localhost", port)}
// 			proxy, err := NewCollectorProxy(
// 				test.grpcBuilder,
// 				nil,
// 				mFactory,
// 				zap.NewNop())

// 			require.NoError(t, err)
// 			require.NotNil(t, proxy)
// 			assert.NotNil(t, proxy.GetReporter())
// 			assert.NotNil(t, proxy.GetManager())
// 			assert.NotNil(t, proxy.GetConn())

// 			r := proxy.GetReporter()

// 			err = r.EmitBatch(&jaeger.Batch{Spans: []*jaeger.Span{{OperationName: "op"}}, Process: &jaeger.Process{ServiceName: "service"}})

// 			if test.expectError {
// 				require.Error(t, err)
// 			} else {
// 				require.NoError(t, err)
// 			}

// 			require.Nil(t, proxy.Close())
// 		})
// 	}
// }
