package grpc

// Copyright (c) 2017 Uber Technologies, Inc.
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
	"google.golang.org/grpc/naming"
	yaml "gopkg.in/yaml.v2"
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

type fakeResolver struct {
}

func (fakeResolver) Resolve(target string) (naming.Watcher, error) {
	return fakeWatcher{}, nil
}

type fakeWatcher struct{}

func (fakeWatcher) Next() ([]*naming.Update, error) {
	return nil, nil
}

func (fakeWatcher) Close() {}

func TestBuilderFromConfig(t *testing.T) {
	cfg := Builder{}
	err := yaml.Unmarshal([]byte(yamlConfig), &cfg)
	require.NoError(t, err)

	assert.Equal(
		t,
		[]string{"127.0.0.1:14267", "127.0.0.1:14268", "127.0.0.1:14269"},
		cfg.CollectorHostPorts)
	r, err := cfg.CreateReporter(zap.NewNop(), metrics.NullFactory, nil)
	require.NoError(t, err)
	assert.NotNil(t, r)

}

func TestBuilderWithCollectors(t *testing.T) {
	tests := []struct {
		target          string
		name            string
		hostPorts       []string
		checkSuffixOnly bool
		resolverTarget  string
		resolver        naming.Resolver
		err             error
	}{
		{
			target:          "///round_robin",
			name:            "with roundrobin schema",
			hostPorts:       []string{"127.0.0.1:9876", "127.0.0.1:9877", "127.0.0.1:9878"},
			checkSuffixOnly: true,
			resolverTarget:  "",
			resolver:        nil,
		},
		{
			target:          "127.0.0.1:9876",
			name:            "with single host",
			hostPorts:       []string{"127.0.0.1:9876"},
			checkSuffixOnly: false,
			resolverTarget:  "",
			resolver:        nil,
		},
		{
			target:          "dns://random_stuff",
			name:            "with custom resolver",
			hostPorts:       []string{},
			checkSuffixOnly: false,
			resolverTarget:  "dns://random_stuff",
			resolver:        fakeResolver{},
		},
		{
			target:          "",
			name:            "without collectorPorts and resolver",
			hostPorts:       nil,
			checkSuffixOnly: false,
			resolverTarget:  "",
			resolver:        nil,
			err:             errors.New("at least one collector hostPort address is required when resolver is not available"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Use NewBuilder for code coverage consideration
			cfg := NewBuilder()
			cfg.CollectorHostPorts = test.hostPorts
			cfg.ResolverTarget = test.resolverTarget
			cfg.Resolver = test.resolver

			agent, err := cfg.CreateReporter(zap.NewNop(), metrics.NullFactory, nil)

			if err != nil {
				assert.Equal(t, test.err, err)
			} else {
				assert.NotNil(t, agent)

				if test.checkSuffixOnly {
					assert.True(t, strings.HasSuffix(agent.conn.Target(), test.target))
				} else {
					assert.True(t, agent.conn.Target() == test.target)
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
		grpcBuilder *Builder
		expectError bool
	}{
		{
			name:        "with insecure grpc connection",
			grpcBuilder: &Builder{CollectorHostPorts: []string{"localhost:0000"}},
			expectError: false,
		},
		{
			name:        "with secure grpc connection",
			grpcBuilder: &Builder{CollectorHostPorts: []string{"localhost:0000"}, TLS: true},
			expectError: false,
		},
		{
			name:        "with secure grpc connection and own CA",
			grpcBuilder: &Builder{CollectorHostPorts: []string{"localhost:0000"}, TLS: true, TLSCA: tmpfile.Name()},
			expectError: false,
		},
		{
			name:        "with secure grpc connection and a CA file which does not exist",
			grpcBuilder: &Builder{CollectorHostPorts: []string{"localhost:0000"}, TLS: true, TLSCA: "/not/valid"},
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
