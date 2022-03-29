// Copyright (c) 2022 The Jaeger Authors.
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
	"reflect"
	"testing"
)

func TestCipherSuiteNamesToIDs(t *testing.T) {
	tests := []struct {
		flag          []string
		expected      []uint16
		expectedError bool
	}{
		{
			// Happy case
			flag:          []string{"TLS_AES_128_GCM_SHA256", "TLS_AES_256_GCM_SHA384", "TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA", "TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384"},
			expected:      []uint16{tls.TLS_AES_128_GCM_SHA256, tls.TLS_AES_256_GCM_SHA384, tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA, tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384},
			expectedError: false,
		},
		{
			// One flag only
			flag:          []string{"TLS_AES_128_GCM_SHA256"},
			expected:      []uint16{tls.TLS_AES_128_GCM_SHA256},
			expectedError: false,
		},
		{
			// Empty flag
			flag:          []string{},
			expected:      nil,
			expectedError: false,
		},
		{
			// Duplicated flag
			flag:          []string{"TLS_AES_128_GCM_SHA256", "TLS_AES_256_GCM_SHA384", "TLS_AES_128_GCM_SHA256"},
			expected:      []uint16{tls.TLS_AES_128_GCM_SHA256, tls.TLS_AES_256_GCM_SHA384, tls.TLS_AES_128_GCM_SHA256},
			expectedError: false,
		},
		{
			// Invalid flag
			flag:          []string{"TLS_INVALID_CIPHER_SUITE"},
			expected:      nil,
			expectedError: true,
		},
	}

	for i, test := range tests {
		uIntFlags, err := CipherSuiteNamesToIDs(test.flag)
		if !reflect.DeepEqual(uIntFlags, test.expected) {
			t.Errorf("%d: expected %+v, got %+v", i, test.expected, uIntFlags)
		}
		if test.expectedError && err == nil {
			t.Errorf("%d: expecting error, got %+v", i, err)
		}
	}
}

func TestVersionNameToID(t *testing.T) {
	tests := []struct {
		flag          string
		expected      uint16
		expectedError bool
	}{
		{
			// Happy case
			flag:          "1.1",
			expected:      tls.VersionTLS11,
			expectedError: false,
		},
		{
			// Invalid flag
			flag:          "Invalid",
			expected:      0,
			expectedError: true,
		},
	}

	for i, test := range tests {
		uIntFlag, err := VersionNameToID(test.flag)
		if !reflect.DeepEqual(uIntFlag, test.expected) {
			t.Errorf("%d: expected %+v, got %+v", i, test.expected, uIntFlag)
		}
		if test.expectedError && err == nil {
			t.Errorf("%d: expecting error, got %+v", i, err)
		}
	}
}
