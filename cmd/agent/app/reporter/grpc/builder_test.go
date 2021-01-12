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
	"context"
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
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
	yaml "gopkg.in/yaml.v2"

	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
	"github.com/jaegertracing/jaeger/pkg/discovery"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
	"github.com/jaegertracing/jaeger/thrift-gen/jaeger"
)

var yamlConfig = `
collectorHostPorts:
    - 127.0.0.1:14268
    - 127.0.0.1:14269
`

var testCertKeyLocation = "../../../../../pkg/config/tlscfg/testdata/"

type noopNotifier struct{}

func (noopNotifier) Register(chan<- []string) {}

func (noopNotifier) Unregister(chan<- []string) {}

func TestBuilderFromConfig(t *testing.T) {
	cfg := ConnBuilder{}
	err := yaml.Unmarshal([]byte(yamlConfig), &cfg)
	require.NoError(t, err)

	assert.Equal(
		t,
		[]string{"127.0.0.1:14268", "127.0.0.1:14269"},
		cfg.CollectorHostPorts)
	r, err := cfg.CreateConnection(zap.NewNop())
	require.NoError(t, err)
	assert.NotNil(t, r)
}

func TestBuilderWithCollectors(t *testing.T) {
	spanHandler1 := &mockSpanHandler{}
	s1, addr1 := initializeGRPCTestServer(t, func(s *grpc.Server) {
		api_v2.RegisterCollectorServiceServer(s, spanHandler1)
	})
	defer s1.Stop()

	tests := []struct {
		target               string
		name                 string
		hostPorts            []string
		checkSuffixOnly      bool
		notifier             discovery.Notifier
		discoverer           discovery.Discoverer
		expectedError        string
		checkConnectionState bool
		expectedState        string
	}{
		{
			target:               "///round_robin",
			name:                 "with roundrobin schema",
			hostPorts:            []string{"127.0.0.1:9876", "127.0.0.1:9877", "127.0.0.1:9878"},
			checkSuffixOnly:      true,
			notifier:             nil,
			discoverer:           nil,
			checkConnectionState: false,
		},
		{
			target:               "127.0.0.1:9876",
			name:                 "with single host",
			hostPorts:            []string{"127.0.0.1:9876"},
			checkSuffixOnly:      false,
			notifier:             nil,
			discoverer:           nil,
			checkConnectionState: false,
		},
		{
			target:               "///round_robin",
			name:                 "with custom resolver and fixed discoverer",
			hostPorts:            []string{"dns://random_stuff"},
			checkSuffixOnly:      true,
			notifier:             noopNotifier{},
			discoverer:           discovery.FixedDiscoverer{},
			checkConnectionState: false,
		},
		{
			target:               "",
			name:                 "without collectorPorts and resolver",
			hostPorts:            nil,
			checkSuffixOnly:      false,
			notifier:             nil,
			discoverer:           nil,
			expectedError:        "at least one collector hostPort address is required when resolver is not available",
			checkConnectionState: false,
		},
		{
			target:               addr1.String(),
			name:                 "with collector connection status ready",
			hostPorts:            []string{addr1.String()},
			checkSuffixOnly:      false,
			notifier:             nil,
			discoverer:           nil,
			checkConnectionState: true,
			expectedState:        "READY",
		},
		{
			target:               "random_stuff",
			name:                 "with collector connection status failure",
			hostPorts:            []string{"random_stuff"},
			checkSuffixOnly:      false,
			notifier:             nil,
			discoverer:           nil,
			checkConnectionState: true,
			expectedState:        "TRANSIENT_FAILURE",
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
				if test.checkConnectionState {
					assertConnectionState(t, conn, test.expectedState)
				}
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
			},
			expectError: false,
		},
		{
			name: "should fail with secure grpc connection and a CA file which does not exist",
			grpcBuilder: &ConnBuilder{
				CollectorHostPorts: []string{"localhost:0000"},
				TLS: tlscfg.Options{
					Enabled: true,
					CAPath:  testCertKeyLocation + "/not/valid",
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
					CAPath:   testCertKeyLocation + "/wrong-CA-cert.pem",
					CertPath: testCertKeyLocation + "/example-client-cert.pem",
					KeyPath:  testCertKeyLocation + "/example-client-key.pem",
				},
			},
			expectError: false,
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
				CertPath: testCertKeyLocation + "/example-server-cert.pem",
				KeyPath:  testCertKeyLocation + "/example-server-key.pem",
			},
			clientTLS: tlscfg.Options{
				Enabled:    true,
				ServerName: "example.com",
			},
			expectError: true,
		},
		{
			name: "should fail with TLS client to trusted TLS server with incorrect hostname",
			serverTLS: tlscfg.Options{
				Enabled:  true,
				CertPath: testCertKeyLocation + "/example-server-cert.pem",
				KeyPath:  testCertKeyLocation + "/example-server-key.pem",
			},
			clientTLS: tlscfg.Options{
				Enabled: true,
				CAPath:  testCertKeyLocation + "/example-CA-cert.pem",
			},
			expectError: true,
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
			expectError: false,
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
			expectError: true,
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
			expectError: true,
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
			expectError: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var opts []grpc.ServerOption
			if test.serverTLS.Enabled {
				tlsCfg, err := test.serverTLS.Config(zap.NewNop())
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
				grpcBuilder,
				nil,
				mFactory,
				zap.NewNop())

			require.NoError(t, err)
			require.NotNil(t, proxy)
			assert.NotNil(t, proxy.GetReporter())
			assert.NotNil(t, proxy.GetManager())
			assert.NotNil(t, proxy.GetConn())

			r := proxy.GetReporter()

			err = r.EmitBatch(context.Background(), &jaeger.Batch{Spans: []*jaeger.Span{{OperationName: "op"}}, Process: &jaeger.Process{ServiceName: "service"}})

			if test.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			require.Nil(t, proxy.Close())
		})
	}
}

func assertConnectionState(t *testing.T, conn *grpc.ClientConn, expectedState string) {
	for {
		s := conn.GetState()
		if s == connectivity.Ready {
			assert.True(t, s.String() == expectedState)
			break
		}
		if s == connectivity.TransientFailure {
			assert.True(t, s.String() == expectedState)
			break
		}
	}
}
