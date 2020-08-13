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

package tlscfg

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

var testCertKeyLocation = "./testdata"

func TestOptionsToConfig(t *testing.T) {
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
			options: Options{CAPath: testCertKeyLocation + "/example-CA-cert.pem"},
		},
		{
			name:        "should fail with invalid CA file path",
			options:     Options{CAPath: testCertKeyLocation + "/not/valid"},
			expectError: "failed to load CA",
		},
		{
			name:        "should fail with invalid CA file content",
			options:     Options{CAPath: testCertKeyLocation + "/bad-CA-cert.txt"},
			expectError: "failed to parse CA",
		},
		{
			name: "should load valid TLS Client settings",
			options: Options{
				CAPath:   testCertKeyLocation + "/example-CA-cert.pem",
				CertPath: testCertKeyLocation + "/example-client-cert.pem",
				KeyPath:  testCertKeyLocation + "/example-client-key.pem",
			},
		},
		{
			name: "should fail with missing TLS Client Key",
			options: Options{
				CAPath:   testCertKeyLocation + "/example-CA-cert.pem",
				CertPath: testCertKeyLocation + "/example-client-cert.pem",
			},
			expectError: "both client certificate and key must be supplied",
		},
		{
			name: "should fail with invalid TLS Client Key",
			options: Options{
				CAPath:   testCertKeyLocation + "/example-CA-cert.pem",
				CertPath: testCertKeyLocation + "/example-client-cert.pem",
				KeyPath:  testCertKeyLocation + "/not/valid",
			},
			expectError: "failed to load server TLS cert and key",
		},
		{
			name: "should fail with missing TLS Client Cert",
			options: Options{
				CAPath:  testCertKeyLocation + "/example-CA-cert.pem",
				KeyPath: testCertKeyLocation + "/example-client-key.pem",
			},
			expectError: "both client certificate and key must be supplied",
		},
		{
			name: "should fail with invalid TLS Client Cert",
			options: Options{
				CAPath:   testCertKeyLocation + "/example-CA-cert.pem",
				CertPath: testCertKeyLocation + "/not/valid",
				KeyPath:  testCertKeyLocation + "/example-client-key.pem",
			},
			expectError: "failed to load server TLS cert and key",
		},
		{
			name: "should fail with invalid TLS Client CA",
			options: Options{
				ClientCAPath: testCertKeyLocation + "/not/valid",
			},
			expectError: "failed to load CA",
		},
		{
			name: "should fail with invalid Client CA pool",
			options: Options{
				ClientCAPath: testCertKeyLocation + "/bad-CA-cert.txt",
			},
			expectError: "failed to parse CA",
		},
		{
			name: "should pass with valid Client CA pool",
			options: Options{
				ClientCAPath: testCertKeyLocation + "/example-CA-cert.pem",
			},
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
			cfg, err := test.options.Config(zap.NewNop())
			if test.expectError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), test.expectError)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, cfg)

				if test.options.CertPath != "" && test.options.KeyPath != "" {
					c, e := tls.LoadX509KeyPair(filepath.Clean(test.options.CertPath), filepath.Clean(test.options.KeyPath))
					require.NoError(t, e)
					cert, err := cfg.GetCertificate(&tls.ClientHelloInfo{})
					require.NoError(t, err)
					assert.Equal(t, &c, cert)
					cert, err = cfg.GetClientCertificate(&tls.CertificateRequestInfo{})
					require.NoError(t, err)
					assert.Equal(t, &c, cert)
				}
			}
			assert.NoError(t, test.options.Close())
		})
	}
}
