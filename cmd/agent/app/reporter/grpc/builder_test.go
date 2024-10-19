// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	yaml "gopkg.in/yaml.v3"

	"github.com/jaegertracing/jaeger/internal/metricstest"
	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
	"github.com/jaegertracing/jaeger/pkg/discovery"
	"github.com/jaegertracing/jaeger/pkg/metrics"
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r, err := cfg.CreateConnection(ctx, zap.NewNop(), metrics.NullFactory)
	require.NoError(t, err)
	defer r.Close()
	assert.NotNil(t, r)
}

func TestBuilderWithCollectors(t *testing.T) {
	spanHandler1 := &mockSpanHandler{}
	s1, _ := initializeGRPCTestServer(t, func(s *grpc.Server) {
		api_v2.RegisterCollectorServiceServer(s, spanHandler1)
	})
	defer s1.Stop()

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

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			conn, err := cfg.CreateConnection(ctx, zap.NewNop(), metrics.NullFactory)
			if test.expectedError == "" {
				require.NoError(t, err)
				defer conn.Close()
				require.NotNil(t, conn)
				if test.checkSuffixOnly {
					assert.True(t, strings.HasSuffix(conn.Target(), test.target))
				} else {
					assert.Equal(t, conn.Target(), test.target)
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			proxy, err := NewCollectorProxy(ctx, test.grpcBuilder, nil, metrics.NullFactory, zap.NewNop())

			if test.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, proxy)

				assert.NotNil(t, proxy.GetReporter())
				assert.NotNil(t, proxy.GetManager())

				require.NoError(t, proxy.Close())
				require.EqualError(t, proxy.Close(), "rpc error: code = Canceled desc = grpc: the client connection is closing")
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var opts []grpc.ServerOption
			if test.serverTLS.Enabled {
				tlsCfg, err := test.serverTLS.Config(zap.NewNop())
				require.NoError(t, err)
				opts = []grpc.ServerOption{grpc.Creds(credentials.NewTLS(tlsCfg))}
			}

			defer test.serverTLS.Close()
			spanHandler := &mockSpanHandler{}
			s, addr := initializeGRPCTestServer(t, func(s *grpc.Server) {
				api_v2.RegisterCollectorServiceServer(s, spanHandler)
			}, opts...)
			defer s.Stop()

			mFactory := metricstest.NewFactory(time.Microsecond)
			defer mFactory.Stop()
			_, port, _ := net.SplitHostPort(addr.String())

			grpcBuilder := &ConnBuilder{
				CollectorHostPorts: []string{net.JoinHostPort("localhost", port)},
				TLS:                test.clientTLS,
			}
			proxy, err := NewCollectorProxy(
				ctx,
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

			err = r.EmitBatch(ctx, &jaeger.Batch{Spans: []*jaeger.Span{{OperationName: "op"}}, Process: &jaeger.Process{ServiceName: "service"}})

			if test.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			require.NoError(t, proxy.Close())
		})
	}
}

type fakeInterceptor struct {
	isCalled bool
}

func (f *fakeInterceptor) intercept(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	f.isCalled = true
	return invoker(ctx, method, req, reply, cc, opts...)
}

func (f *fakeInterceptor) assertCalled(t *testing.T) {
	t.Helper()
	assert.True(t, f.isCalled)
}

func TestBuilderWithAdditionalDialOptions(t *testing.T) {
	fi := fakeInterceptor{}
	defer fi.assertCalled(t)

	cb := ConnBuilder{
		CollectorHostPorts:    []string{"127.0.0.1:14268"},
		AdditionalDialOptions: []grpc.DialOption{grpc.WithUnaryInterceptor(fi.intercept)},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r, err := cb.CreateConnection(ctx, zap.NewNop(), metrics.NullFactory)
	require.NoError(t, err)
	defer r.Close()
	assert.NotNil(t, r)

	err = r.Invoke(context.Background(), "test", map[string]string{}, map[string]string{}, []grpc.CallOption{}...)
	require.Error(t, err, "should error because no server is running")
}
