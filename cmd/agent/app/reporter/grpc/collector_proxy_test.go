// Copyright (c) 2018 The Jaeger Authors.
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
	"crypto/x509"
	"errors"
	"io/ioutil"
	"net"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics"
	"github.com/uber/jaeger-lib/metrics/metricstest"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
	"github.com/jaegertracing/jaeger/thrift-gen/jaeger"
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

func TestProxyBuilderMissingAddress(t *testing.T) {
	proxy, err := NewCollectorProxy(&Options{}, nil, metrics.NullFactory, zap.NewNop())
	require.Nil(t, proxy)
	assert.EqualError(t, err, "could not create collector proxy, address is missing")
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
		name         string
		proxyOptions *Options
		expectError  bool
	}{
		{
			name:         "with insecure grpc connection",
			proxyOptions: &Options{CollectorHostPort: []string{"localhost:0000"}},
			expectError:  false,
		},
		{
			name:         "with secure grpc connection",
			proxyOptions: &Options{CollectorHostPort: []string{"localhost:0000"}, TLS: true},
			expectError:  false,
		},
		{
			name:         "with secure grpc connection and own CA",
			proxyOptions: &Options{CollectorHostPort: []string{"localhost:0000"}, TLS: true, TLSCA: tmpfile.Name()},
			expectError:  false,
		},
		{
			name:         "with secure grpc connection and a CA file which does not exist",
			proxyOptions: &Options{CollectorHostPort: []string{"localhost:0000"}, TLS: true, TLSCA: "/not/valid"},
			expectError:  true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			proxy, err := NewCollectorProxy(test.proxyOptions, nil, metrics.NullFactory, zap.NewNop())
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

// This test is only for coverage.
func TestSystemCertPoolError(t *testing.T) {
	fakeErr := errors.New("fake error")
	systemCertPool = func() (*x509.CertPool, error) {
		return nil, fakeErr
	}
	_, err := NewCollectorProxy(&Options{
		CollectorHostPort: []string{"foo", "bar"},
		TLS:               true,
	}, nil, nil, nil)
	assert.Equal(t, fakeErr, err)
}

func TestMultipleCollectors(t *testing.T) {
	spanHandler1 := &mockSpanHandler{}
	s1, addr1 := initializeGRPCTestServer(t, func(s *grpc.Server) {
		api_v2.RegisterCollectorServiceServer(s, spanHandler1)
	})
	defer s1.Stop()
	spanHandler2 := &mockSpanHandler{}
	s2, addr2 := initializeGRPCTestServer(t, func(s *grpc.Server) {
		api_v2.RegisterCollectorServiceServer(s, spanHandler2)
	})
	defer s2.Stop()

	mFactory := metricstest.NewFactory(time.Microsecond)
	proxy, err := NewCollectorProxy(&Options{CollectorHostPort: []string{addr1.String(), addr2.String()}}, nil, mFactory, zap.NewNop())
	require.NoError(t, err)
	require.NotNil(t, proxy)
	assert.NotNil(t, proxy.GetReporter())
	assert.NotNil(t, proxy.GetManager())

	var bothServers = false
	r := proxy.GetReporter()
	// TODO do not iterate, just create two batches
	for i := 0; i < 100; i++ {
		err := r.EmitBatch(&jaeger.Batch{Spans: []*jaeger.Span{{OperationName: "op"}}, Process: &jaeger.Process{ServiceName: "service"}})
		require.NoError(t, err)
		if len(spanHandler1.getRequests()) > 0 && len(spanHandler2.getRequests()) > 0 {
			bothServers = true
			break
		}
	}
	c, g := mFactory.Snapshot()
	assert.True(t, len(g) > 0)
	assert.True(t, len(c) > 0)
	assert.Equal(t, true, bothServers)
	require.Nil(t, proxy.Close())
}

func initializeGRPCTestServer(t *testing.T, beforeServe func(server *grpc.Server)) (*grpc.Server, net.Addr) {
	server := grpc.NewServer()
	lis, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	beforeServe(server)
	go func() {
		require.NoError(t, server.Serve(lis))
	}()
	return server, lis.Addr()
}
