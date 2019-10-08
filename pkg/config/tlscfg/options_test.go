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
			expectError: "failed to load CA",
		},
		{
			name:        "should fail with invalid CA file content",
			options:     Options{CAPath: "testdata/testCA-bad.txt"},
			expectError: "failed to parse CA",
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
		{
			name: "should fail with invalid TLS Client CA",
			options: Options{
				ClientCAPath: "testdata/not/valid",
			},
			expectError: "failed to load CA",
		},
		{
			name: "should fail with invalid Client CA pool",
			options: Options{
				ClientCAPath: "testdata/testCA-bad.txt",
			},
			expectError: "failed to parse CA",
		},
		{
			name: "should pass with valid Client CA pool",
			options: Options{
				ClientCAPath: "testdata/testCA.pem",
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
			cfg, err := test.options.Config()
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
