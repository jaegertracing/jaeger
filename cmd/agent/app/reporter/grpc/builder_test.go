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
	"net"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics"
	"github.com/uber/jaeger-lib/metrics/metricstest"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	yaml "gopkg.in/yaml.v2"

	"github.com/jaegertracing/jaeger/cmd/agent/app/reporter"
	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
	"github.com/jaegertracing/jaeger/pkg/discovery"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
	"github.com/jaegertracing/jaeger/thrift-gen/jaeger"
)

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
		expectedError   string
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
			target:          "///round_robin",
			name:            "with custom resolver and fixed discoverer",
			hostPorts:       []string{"dns://random_stuff"},
			checkSuffixOnly: true,
			notifier:        noopNotifier{},
			discoverer:      discovery.FixedDiscoverer{},
		},
		{
			target:          "",
			name:            "without collectorPorts and resolver",
			hostPorts:       nil,
			checkSuffixOnly: false,
			notifier:        nil,
			discoverer:      nil,
			expectedError:   "at least one collector hostPort address is required when resolver is not available",
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
			if test.expectedError == "" {
				require.NoError(t, err)
				require.NotNil(t, conn)

				if test.checkSuffixOnly {
					assert.True(t, strings.HasSuffix(conn.Target(), test.target))
				} else {
					assert.True(t, conn.Target() == test.target)
				}
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), test.expectedError)
			}
		})
	}
}

func TestProxyBuilder(t *testing.T) {
	tests := []struct {
		name        string
		grpcBuilder *ConnBuilder
		expectError bool
	}{
		{
			name: "should pass with insecure grpc connection",
			grpcBuilder: &ConnBuilder{
				CollectorHostPorts: []string{"localhost:0000"},
				TLS:                true,
				TLSCA:              "testdata/testCA.pem",
				TLSCert:            "testdata/client.jaeger.io-client.pem",
				TLSKey:             "testdata/client.jaeger.io-client-key.pem",
			},
			expectError: false,
		},
		{
			name: "should fail with secure grpc connection and a CA file which does not exist",
			grpcBuilder: &ConnBuilder{
				CollectorHostPorts: []string{"localhost:0000"},
				TLS: tlscfg.Options{
					Enabled: true,
					CAPath:  "testdata/not/valid",
				},
			},
			expectError: true,
		},
		{
			name: "should pass with secure grpc connection and valid TLS Client settings",
			grpcBuilder: &ConnBuilder{
				CollectorHostPorts: []string{"localhost:0000"},
				TLS: tlscfg.Options{
					Enabled:  true,
					CAPath:   "testdata/testCA.pem",
					CertPath: "testdata/client.jaeger.io-client.pem",
					KeyPath:  "testdata/client.jaeger.io-client-key.pem",
				},
			},
			expectError: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			proxy, err := NewCollectorProxy(test.grpcBuilder, &reporter.Options{}, metrics.NullFactory, zap.NewNop())
			if test.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, proxy)

				assert.NotNil(t, proxy.GetReporter())
				assert.NotNil(t, proxy.GetManager())

				assert.Nil(t, proxy.Close())
			}
		})
	}
}

func TestProxyClientTLS(t *testing.T) {
	tests := []struct {
		name        string
		clientTLS   tlscfg.Options
		serverTLS   tlscfg.Options
		expectError bool
	}{
		{
			name:        "should pass with insecure grpc connection",
			expectError: false,
		},
		{
			name:        "should fail with TLS client to non-TLS server",
			clientTLS:   tlscfg.Options{Enabled: true},
			expectError: true,
		},
		{
			name: "should fail with TLS client to untrusted TLS server",
			serverTLS: tlscfg.Options{
				Enabled:  true,
				CertPath: "testdata/server.jaeger.io.pem",
				KeyPath:  "testdata/server.jaeger.io-key.pem",
			},
			clientTLS: tlscfg.Options{
				Enabled:    true,
				ServerName: "server.jaeger.io",
			},
			expectError: true,
		},
		{
			name: "should fail with TLS client to trusted TLS server with incorrect hostname",
			serverTLS: tlscfg.Options{
				Enabled:  true,
				CertPath: "testdata/server.jaeger.io.pem",
				KeyPath:  "testdata/server.jaeger.io-key.pem",
			},
			clientTLS: tlscfg.Options{
				Enabled: true,
				CAPath:  "testdata/rootCA.pem",
			},
			expectError: true,
		},
		{
			name: "should pass with TLS client to trusted TLS server with correct hostname",
			serverTLS: tlscfg.Options{
				Enabled:  true,
				CertPath: "testdata/server.jaeger.io.pem",
				KeyPath:  "testdata/server.jaeger.io-key.pem",
			},
			clientTLS: tlscfg.Options{
				Enabled:    true,
				CAPath:     "testdata/rootCA.pem",
				ServerName: "server.jaeger.io",
			},
			expectError: false,
		},
		{
			name: "should fail with TLS client without cert to trusted TLS server requiring cert",
			serverTLS: tlscfg.Options{
				Enabled:      true,
				CertPath:     "testdata/server.jaeger.io.pem",
				KeyPath:      "testdata/server.jaeger.io-key.pem",
				ClientCAPath: "testdata/rootCA.pem",
			},
			clientTLS: tlscfg.Options{
				Enabled:    true,
				CAPath:     "testdata/rootCA.pem",
				ServerName: "server.jaeger.io",
			},
			expectError: true,
		},
		{
			name: "should fail with TLS client without cert to trusted TLS server requiring cert from a different CA",
			serverTLS: tlscfg.Options{
				Enabled:      true,
				CertPath:     "testdata/server.jaeger.io.pem",
				KeyPath:      "testdata/server.jaeger.io-key.pem",
				ClientCAPath: "testdata/testCA.pem", // NB: wrong CA
			},
			clientTLS: tlscfg.Options{
				Enabled:    true,
				CAPath:     "testdata/rootCA.pem",
				ServerName: "server.jaeger.io",
				CertPath:   "testdata/client.jaeger.io-client.pem",
				KeyPath:    "testdata/client.jaeger.io-client-key.pem",
			},
			expectError: true,
		},
		{
			name: "should pass with TLS client with cert to trusted TLS server requiring cert",
			serverTLS: tlscfg.Options{
				Enabled:      true,
				CertPath:     "testdata/server.jaeger.io.pem",
				KeyPath:      "testdata/server.jaeger.io-key.pem",
				ClientCAPath: "testdata/rootCA.pem",
			},
			clientTLS: tlscfg.Options{
				Enabled:    true,
				CAPath:     "testdata/rootCA.pem",
				ServerName: "server.jaeger.io",
				CertPath:   "testdata/client.jaeger.io-client.pem",
				KeyPath:    "testdata/client.jaeger.io-client-key.pem",
			},
			expectError: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var opts []grpc.ServerOption
			if test.serverTLS.Enabled {
				tlsCfg, err := test.serverTLS.Config()
				require.NoError(t, err)
				opts = []grpc.ServerOption{grpc.Creds(credentials.NewTLS(tlsCfg))}
			}

			spanHandler := &mockSpanHandler{}
			s, addr := initializeGRPCTestServer(t, func(s *grpc.Server) {
				api_v2.RegisterCollectorServiceServer(s, spanHandler)
			}, opts...)
			defer s.Stop()

			mFactory := metricstest.NewFactory(time.Microsecond)
			_, port, _ := net.SplitHostPort(addr.String())

			grpcBuilder := &ConnBuilder{
				CollectorHostPorts: []string{net.JoinHostPort("localhost", port)},
				TLS:                test.clientTLS,
			}
			proxy, err := NewCollectorProxy(
				test.grpcBuilder,
				&reporter.Options{QueueType: reporter.DIRECT},
				mFactory,
				zap.NewNop())

			require.NoError(t, err)
			require.NotNil(t, proxy)
			assert.NotNil(t, proxy.GetReporter())
			assert.NotNil(t, proxy.GetManager())
			assert.NotNil(t, proxy.GetConn())

			r := proxy.GetReporter()

			err = r.EmitBatch(&jaeger.Batch{Spans: []*jaeger.Span{{OperationName: "op"}}, Process: &jaeger.Process{ServiceName: "service"}})

			if test.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			require.Nil(t, proxy.Close())
		})
	}
}
