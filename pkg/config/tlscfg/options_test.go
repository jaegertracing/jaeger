// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tlscfg

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configtls"
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
			name: "should fail with invalid TLS Client CA pool",
			options: Options{
				ClientCAPath: testCertKeyLocation + "/bad-CA-cert.txt",
			},
			expectError: "failed to parse CA",
		},
		{
			name: "should pass with valid TLS Client CA pool",
			options: Options{
				ClientCAPath: testCertKeyLocation + "/example-CA-cert.pem",
			},
		},
		{
			name: "should fail with invalid TLS Cipher Suite",
			options: Options{
				CipherSuites: []string{"TLS_INVALID_CIPHER_SUITE"},
			},
			expectError: "failed to get cipher suite ids from cipher suite names: cipher suite TLS_INVALID_CIPHER_SUITE not supported or doesn't exist",
		},
		{
			name: "should fail with invalid TLS Min Version",
			options: Options{
				MinVersion: "Invalid",
			},
			expectError: "failed to get minimum tls version",
		},
		{
			name: "should fail with invalid TLS Max Version",
			options: Options{
				MaxVersion: "Invalid",
			},
			expectError: "failed to get maximum tls version",
		},
		{
			name: "should fail with TLS Min Version greater than TLS Max Version error",
			options: Options{
				MinVersion: "1.2",
				MaxVersion: "1.1",
			},
			expectError: "minimum tls version can't be greater than maximum tls version",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.fakeSysPool {
				saveSystemCertPool := systemCertPool
				systemCertPool = func() (*x509.CertPool, error) {
					return nil, errors.New("fake system pool")
				}
				defer func() {
					systemCertPool = saveSystemCertPool
				}()
			}
			cfg, err := test.options.Config(zap.NewNop())
			if test.expectError != "" {
				require.ErrorContains(t, err, test.expectError)
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
			require.NoError(t, test.options.close())
		})
	}
}

func TestToOtelClientConfig(t *testing.T) {
	testCases := []struct {
		name     string
		options  Options
		expected configtls.ClientConfig
	}{
		{
			name: "insecure",
			options: Options{
				Enabled: false,
			},
			expected: configtls.ClientConfig{
				Insecure: true,
			},
		},
		{
			name: "secure with skip host verify",
			options: Options{
				Enabled:        true,
				SkipHostVerify: true,
				ServerName:     "example.com",
				CAPath:         "path/to/ca.pem",
				CertPath:       "path/to/cert.pem",
				KeyPath:        "path/to/key.pem",
				CipherSuites:   []string{"TLS_RSA_WITH_AES_128_CBC_SHA"},
				MinVersion:     "1.2",
				MaxVersion:     "1.3",
				ReloadInterval: 24 * time.Hour,
			},
			expected: configtls.ClientConfig{
				Insecure:           false,
				InsecureSkipVerify: true,
				ServerName:         "example.com",
				Config: configtls.Config{
					CAFile:         "path/to/ca.pem",
					CertFile:       "path/to/cert.pem",
					KeyFile:        "path/to/key.pem",
					CipherSuites:   []string{"TLS_RSA_WITH_AES_128_CBC_SHA"},
					MinVersion:     "1.2",
					MaxVersion:     "1.3",
					ReloadInterval: 24 * time.Hour,
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := tc.options.ToOtelClientConfig()
			assert.Equal(t, tc.expected, actual)
		})
	}
}

func TestToOtelServerConfig(t *testing.T) {
	testCases := []struct {
		name     string
		options  Options
		expected *configtls.ServerConfig
	}{
		{
			name: "not enabled",
			options: Options{
				Enabled: false,
			},
			expected: nil,
		},
		{
			name: "default mapping",
			options: Options{
				Enabled:      true,
				ClientCAPath: "path/to/client/ca.pem",
				CAPath:       "path/to/ca.pem",
				CertPath:     "path/to/cert.pem",
				KeyPath:      "path/to/key.pem",
				CipherSuites: []string{"TLS_RSA_WITH_AES_128_CBC_SHA"},
				MinVersion:   "1.2",
				MaxVersion:   "1.3",
			},
			expected: &configtls.ServerConfig{
				ClientCAFile: "path/to/client/ca.pem",
				Config: configtls.Config{
					CAFile:       "path/to/ca.pem",
					CertFile:     "path/to/cert.pem",
					KeyFile:      "path/to/key.pem",
					CipherSuites: []string{"TLS_RSA_WITH_AES_128_CBC_SHA"},
					MinVersion:   "1.2",
					MaxVersion:   "1.3",
				},
			},
		},
		{
			name: "with reload interval",
			options: Options{
				Enabled:        true,
				ClientCAPath:   "path/to/client/ca.pem",
				CAPath:         "path/to/ca.pem",
				CertPath:       "path/to/cert.pem",
				KeyPath:        "path/to/key.pem",
				CipherSuites:   []string{"TLS_RSA_WITH_AES_128_CBC_SHA"},
				MinVersion:     "1.2",
				MaxVersion:     "1.3",
				ReloadInterval: 24 * time.Hour,
			},
			expected: &configtls.ServerConfig{
				ClientCAFile:       "path/to/client/ca.pem",
				ReloadClientCAFile: true,
				Config: configtls.Config{
					CAFile:         "path/to/ca.pem",
					CertFile:       "path/to/cert.pem",
					KeyFile:        "path/to/key.pem",
					CipherSuites:   []string{"TLS_RSA_WITH_AES_128_CBC_SHA"},
					MinVersion:     "1.2",
					MaxVersion:     "1.3",
					ReloadInterval: 24 * time.Hour,
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := tc.options.ToOtelServerConfig()
			assert.Equal(t, tc.expected, actual)
		})
	}
}
