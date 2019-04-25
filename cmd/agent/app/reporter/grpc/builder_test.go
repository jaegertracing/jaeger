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
package grpc

import (
	"errors"
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"
	yaml "gopkg.in/yaml.v2"

	"github.com/jaegertracing/jaeger/pkg/discovery"
)

const certPEM = `
-----BEGIN CERTIFICATE-----
MIICBzCCAXCgAwIBAgIQNkTaUtOczDHvL2YT/kqScTANBgkqhkiG9w0BAQsFADAX
MRUwEwYDVQQKEwxqYWdlcnRyYWNpbmcwHhcNMTkwMjA4MDYyODAyWhcNMTkwMjA4
MDcyODAyWjAXMRUwEwYDVQQKEwxqYWdlcnRyYWNpbmcwgZ8wDQYJKoZIhvcNAQEB
BQADgY0AMIGJAoGBAMcOLYflHGbqC1f7+tbnsdfcpd0rEuX65+ab0WzelAgvo988
yD+j7LDLPIE8IPk/tfqaETZ8h0LRUUTn8F2rW/wgrl/G8Onz0utog38N0elfTifG
Mu7GJCr/+aYM5xbQMDj4Brb4vhnkJF8UBe49fWILhIltUcm1SeKqVX3d1FvpAgMB
AAGjVDBSMA4GA1UdDwEB/wQEAwICpDATBgNVHSUEDDAKBggrBgEFBQcDATAPBgNV
HRMBAf8EBTADAQH/MBoGA1UdEQQTMBGCCWxvY2FsaG9zdIcEfwAAATANBgkqhkiG
9w0BAQsFAAOBgQCreFjwpAn1HqJT812JOwoWKrt1NjOKGcz7pvIs1k3DfQVLH2aZ
iPKnCkzNgxMzQtwdgpAOXIAqXyNibvyOAv1C+3QSMLKbuPEHaIxlCuvl1suX/g25
17x1o3Q64AnPCWOLpN2wjkfZqX7gZ84nsxpqb9Sbw1+2+kqX7dSZ3mfVxQ==
-----END CERTIFICATE-----`

var yamlConfig = `
collectorHostPorts:
    - 127.0.0.1:14267
    - 127.0.0.1:14268
    - 127.0.0.1:14269
`

type noopNotifier struct{}

func (noopNotifier) Register(chan<- []string) {}

func (noopNotifier) Unregister(chan<- []string) {}

func TestBuilderFromConfig(t *testing.T) {
	cfg := ConnBuilder{}
	err := yaml.Unmarshal([]byte(yamlConfig), &cfg)
	require.NoError(t, err)

	assert.Equal(
		t,
		[]string{"127.0.0.1:14267", "127.0.0.1:14268", "127.0.0.1:14269"},
		cfg.CollectorHostPorts)
	r, err := cfg.CreateConnection(zap.NewNop())
	require.NoError(t, err)
	assert.NotNil(t, r)
}

func TestBuilderWithCollectors(t *testing.T) {
	tests := []struct {
		target          string
		name            string
		hostPorts       []string
		checkSuffixOnly bool
		notifier        discovery.Notifier
		discoverer      discovery.Discoverer
		err             error
	}{
		{
			target:          "///round_robin",
			name:            "with roundrobin schema",
			hostPorts:       []string{"127.0.0.1:9876", "127.0.0.1:9877", "127.0.0.1:9878"},
			checkSuffixOnly: true,
			notifier:        nil,
			discoverer:      nil,
		},
		{
			target:          "127.0.0.1:9876",
			name:            "with single host",
			hostPorts:       []string{"127.0.0.1:9876"},
			checkSuffixOnly: false,
			notifier:        nil,
			discoverer:      nil,
		},
		{
			target:          "dns://random_stuff",
			name:            "with custom resolver",
			hostPorts:       []string{"dns://random_stuff"},
			checkSuffixOnly: false,
			notifier:        noopNotifier{},
			discoverer:      discovery.FixedDiscoverer{},
		},
		{
			target:          "dns://random_stuff",
			name:            "with custom resolver",
			hostPorts:       []string{"dns://random_stuff"},
			checkSuffixOnly: false,
			notifier:        noopNotifier{},
			discoverer:      discovery.ErrorDiscoverer{},
			err:             errors.New("Error discoverer always return error"),
		},
		{
			target:          "",
			name:            "without collectorPorts and resolver",
			hostPorts:       nil,
			checkSuffixOnly: false,
			notifier:        nil,
			discoverer:      nil,
			err:             errors.New("at least one collector hostPort address is required when resolver is not available"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Use NewBuilder for code coverage consideration
			cfg := NewConnBuilder()
			cfg.CollectorHostPorts = test.hostPorts
			cfg.Notifier = test.notifier
			cfg.Discoverer = test.discoverer

			conn, err := cfg.CreateConnection(zap.NewNop())
			if err != nil {
				assert.Equal(t, test.err, err)
			} else {
				assert.NotNil(t, conn)

				if test.checkSuffixOnly {
					assert.True(t, strings.HasSuffix(conn.Target(), test.target))
				} else {
					assert.True(t, conn.Target() == test.target)
				}
			}
		})
	}
}

func TestProxyBuilder(t *testing.T) {
	tmpfile, err := ioutil.TempFile("", "cert*.pem")
	if err != nil {
		t.Fatalf("failed to create tempfile: %s", err)
	}

	defer func() {
		tmpfile.Close()
		os.Remove(tmpfile.Name())
	}()

	if _, err := tmpfile.Write([]byte(certPEM)); err != nil {
		t.Fatalf("failed to write cert to tempfile: %s", err)
	}

	tests := []struct {
		name        string
		grpcBuilder *ConnBuilder
		expectError bool
	}{
		{
			name:        "with insecure grpc connection",
			grpcBuilder: &ConnBuilder{CollectorHostPorts: []string{"localhost:0000"}},
			expectError: false,
		},
		{
			name:        "with secure grpc connection",
			grpcBuilder: &ConnBuilder{CollectorHostPorts: []string{"localhost:0000"}, TLS: true},
			expectError: false,
		},
		{
			name:        "with secure grpc connection and own CA",
			grpcBuilder: &ConnBuilder{CollectorHostPorts: []string{"localhost:0000"}, TLS: true, TLSCA: tmpfile.Name()},
			expectError: false,
		},
		{
			name:        "with secure grpc connection and a CA file which does not exist",
			grpcBuilder: &ConnBuilder{CollectorHostPorts: []string{"localhost:0000"}, TLS: true, TLSCA: "/not/valid"},
			expectError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			proxy, err := NewCollectorProxy(test.grpcBuilder, nil, metrics.NullFactory, zap.NewNop())
			if test.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, proxy)

				assert.NotNil(t, proxy.GetReporter())
				assert.NotNil(t, proxy.GetManager())

				assert.Nil(t, proxy.Close())
				assert.EqualError(t, proxy.Close(), "rpc error: code = Canceled desc = grpc: the client connection is closing")
			}
		})
	}
}
