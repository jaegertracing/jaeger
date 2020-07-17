// Copyright (c) 2019,2020 The Jaeger Authors.
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

package app

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/jaegertracing/jaeger/cmd/flags"
	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
	"github.com/jaegertracing/jaeger/pkg/healthcheck"
	"github.com/jaegertracing/jaeger/ports"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
	depsmocks "github.com/jaegertracing/jaeger/storage/dependencystore/mocks"
	spanstoremocks "github.com/jaegertracing/jaeger/storage/spanstore/mocks"
)

var testCertKeyLocation = "../../../pkg/config/tlscfg/testdata"

func TestServerError(t *testing.T) {
	srv := &Server{
		queryOptions: &QueryOptions{
			HostPort: ":-1",
		},
	}
	assert.Error(t, srv.Start())
}

func TestCreateTLSGrpcServerError(t *testing.T) {
	tlsCfg := tlscfg.Options{
		Enabled:      true,
		CertPath:     "invalid/path",
		KeyPath:      "invalid/path",
		ClientCAPath: "invalid/path",
	}

	_, err := NewServer(zap.NewNop(), &querysvc.QueryService{},
		&QueryOptions{TLSGRPC: tlsCfg}, opentracing.NoopTracer{})
	assert.NotNil(t, err)
}

func TestCreateTLSHttpServerError(t *testing.T) {
	tlsCfg := tlscfg.Options{
		Enabled:      true,
		CertPath:     "invalid/path",
		KeyPath:      "invalid/path",
		ClientCAPath: "invalid/path",
	}

	_, err := NewServer(zap.NewNop(), &querysvc.QueryService{},
		&QueryOptions{TLSHTTP: tlsCfg}, opentracing.NoopTracer{})
	assert.NotNil(t, err)
}

var testCases = []struct {
	name              string
	HTTPTLS           tlscfg.Options
	GRPCTLS           tlscfg.Options
	clientTLS         tlscfg.Options
	expectError       bool
	expectClientError bool
	expectServerFail  bool
}{
	{
		name: "Should pass with insecure connection",
		HTTPTLS: tlscfg.Options{
			Enabled: false,
		},
		GRPCTLS: tlscfg.Options{
			Enabled: false,
		},
		clientTLS: tlscfg.Options{
			Enabled: false,
		},
		expectError:       false,
		expectClientError: false,
		expectServerFail:  false,
	},
	{
		name: "should fail with TLS client to untrusted TLS server",
		HTTPTLS: tlscfg.Options{
			Enabled:  true,
			CertPath: testCertKeyLocation + "/example-server-cert.pem",
			KeyPath:  testCertKeyLocation + "/example-server-key.pem",
		},
		GRPCTLS: tlscfg.Options{
			Enabled:  true,
			CertPath: testCertKeyLocation + "/example-server-cert.pem",
			KeyPath:  testCertKeyLocation + "/example-server-key.pem",
		},
		clientTLS: tlscfg.Options{
			Enabled:    true,
			ServerName: "example.com",
		},
		expectError:       true,
		expectClientError: true,
		expectServerFail:  false,
	},
	{
		name: "should fail with TLS client to trusted TLS server with incorrect hostname",
		HTTPTLS: tlscfg.Options{
			Enabled:  true,
			CertPath: testCertKeyLocation + "/example-server-cert.pem",
			KeyPath:  testCertKeyLocation + "/example-server-key.pem",
		},
		GRPCTLS: tlscfg.Options{
			Enabled:  true,
			CertPath: testCertKeyLocation + "/example-server-cert.pem",
			KeyPath:  testCertKeyLocation + "/example-server-key.pem",
		},
		clientTLS: tlscfg.Options{
			Enabled:    true,
			CAPath:     testCertKeyLocation + "/example-CA-cert.pem",
			ServerName: "nonEmpty",
		},
		expectError:       true,
		expectClientError: true,
		expectServerFail:  false,
	},
	{
		name: "should pass with TLS client to trusted TLS server with correct hostname",
		HTTPTLS: tlscfg.Options{
			Enabled:  true,
			CertPath: testCertKeyLocation + "/example-server-cert.pem",
			KeyPath:  testCertKeyLocation + "/example-server-key.pem",
		},
		GRPCTLS: tlscfg.Options{
			Enabled:  true,
			CertPath: testCertKeyLocation + "/example-server-cert.pem",
			KeyPath:  testCertKeyLocation + "/example-server-key.pem",
		},
		clientTLS: tlscfg.Options{
			Enabled:    true,
			CAPath:     testCertKeyLocation + "/example-CA-cert.pem",
			ServerName: "example.com",
		},
		expectError:       false,
		expectClientError: false,
		expectServerFail:  false,
	},
	{
		name: "should fail with TLS client without cert to trusted TLS server requiring cert",
		HTTPTLS: tlscfg.Options{
			Enabled:      true,
			CertPath:     testCertKeyLocation + "/example-server-cert.pem",
			KeyPath:      testCertKeyLocation + "/example-server-key.pem",
			ClientCAPath: testCertKeyLocation + "/example-CA-cert.pem",
		},
		GRPCTLS: tlscfg.Options{
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
		expectError:       false,
		expectServerFail:  false,
		expectClientError: true,
	},
	{
		name: "should pass with TLS client with cert to trusted TLS server requiring cert",
		HTTPTLS: tlscfg.Options{
			Enabled:      true,
			CertPath:     testCertKeyLocation + "/example-server-cert.pem",
			KeyPath:      testCertKeyLocation + "/example-server-key.pem",
			ClientCAPath: testCertKeyLocation + "/example-CA-cert.pem",
		},
		GRPCTLS: tlscfg.Options{
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
		expectError:       false,
		expectServerFail:  false,
		expectClientError: false,
	},
	{
		name: "should fail with TLS client without cert to trusted TLS server requiring cert from a different CA",
		HTTPTLS: tlscfg.Options{
			Enabled:      true,
			CertPath:     testCertKeyLocation + "/example-server-cert.pem",
			KeyPath:      testCertKeyLocation + "/example-server-key.pem",
			ClientCAPath: testCertKeyLocation + "/wrong-CA-cert.pem", // NB: wrong CA
		},
		GRPCTLS: tlscfg.Options{
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
		expectError:       false,
		expectServerFail:  false,
		expectClientError: true,
	},
	{
		name: "should pass with TLS client with cert to trusted TLS HTTP server requiring cert and insecure GRPC server",
		HTTPTLS: tlscfg.Options{
			Enabled:      true,
			CertPath:     testCertKeyLocation + "/example-server-cert.pem",
			KeyPath:      testCertKeyLocation + "/example-server-key.pem",
			ClientCAPath: testCertKeyLocation + "/example-CA-cert.pem",
		},
		GRPCTLS: tlscfg.Options{
			Enabled: false,
		},
		clientTLS: tlscfg.Options{
			Enabled:    true,
			CAPath:     testCertKeyLocation + "/example-CA-cert.pem",
			ServerName: "example.com",
			CertPath:   testCertKeyLocation + "/example-client-cert.pem",
			KeyPath:    testCertKeyLocation + "/example-client-key.pem",
		},
		expectError:       false,
		expectServerFail:  false,
		expectClientError: false,
	},
	{
		name: "should pass with TLS client with cert to trusted GRPC TLS server requiring cert and insecure HTTP server",
		HTTPTLS: tlscfg.Options{
			Enabled: false,
		},
		GRPCTLS: tlscfg.Options{
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
		expectError:       false,
		expectServerFail:  false,
		expectClientError: false,
	},
}

func TestServerHTTPTLS(t *testing.T) {
	testlen := len(testCases)

	tests := make([]struct {
		name              string
		HTTPTLS           tlscfg.Options
		GRPCTLS           tlscfg.Options
		clientTLS         tlscfg.Options
		expectError       bool
		expectClientError bool
		expectServerFail  bool
	}, testlen)
	copy(tests, testCases)

	tests[testlen-1].clientTLS = tlscfg.Options{Enabled: false}
	tests[testlen-1].name = "Should pass with insecure HTTP Client and insecure HTTP server with secure GRPC Server"

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			serverOptions := &QueryOptions{TLSHTTP: test.HTTPTLS, TLSGRPC: test.GRPCTLS, HostPort: ports.PortToHostPort(ports.QueryHTTP), BearerTokenPropagation: true}
			flagsSvc := flags.NewService(ports.QueryAdminHTTP)
			flagsSvc.Logger = zap.NewNop()

			spanReader := &spanstoremocks.Reader{}
			dependencyReader := &depsmocks.Reader{}
			expectedServices := []string{"test"}
			spanReader.On("GetServices", mock.AnythingOfType("*context.valueCtx")).Return(expectedServices, nil)

			querySvc := querysvc.NewQueryService(spanReader, dependencyReader, querysvc.QueryServiceOptions{})
			server, err := NewServer(flagsSvc.Logger, querySvc,
				serverOptions,
				opentracing.NoopTracer{})
			assert.Nil(t, err)
			assert.NoError(t, server.Start())
			go func() {
				for s := range server.HealthCheckStatus() {
					flagsSvc.SetHealthCheckStatus(s)
				}
			}()

			var clientError error
			var clientClose func() error
			var clientTLSCfg *tls.Config

			if serverOptions.TLSHTTP.Enabled {

				var err0 error

				clientTLSCfg, err0 = test.clientTLS.Config()
				require.NoError(t, err0)
				dialer := &net.Dialer{Timeout: 2 * time.Second}
				conn, err1 := tls.DialWithDialer(dialer, "tcp", "localhost:"+fmt.Sprintf("%d", ports.QueryHTTP), clientTLSCfg)
				clientError = err1
				clientClose = nil
				if conn != nil {
					clientClose = conn.Close
				}

			} else {

				conn, err1 := net.DialTimeout("tcp", "localhost:"+fmt.Sprintf("%d", ports.QueryHTTP), 2*time.Second)
				clientError = err1
				clientClose = nil
				if conn != nil {
					clientClose = conn.Close
				}
			}

			if test.expectError {
				require.Error(t, clientError)
			} else {
				require.NoError(t, clientError)
			}
			if clientClose != nil {
				require.Nil(t, clientClose())
			}

			// defer server.Close()
			// fmt.Print(test.HTTPTLS.ClientCAPath == "" ); fmt.Println(test.HTTPTLS.ClientCAPath)

			if test.HTTPTLS.ClientCAPath != "" {
				client := &http.Client{
					Transport: &http.Transport{
						TLSClientConfig: clientTLSCfg,
					},
				}
				readMock := spanReader
				readMock.On("FindTraces", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("*spanstore.TraceQueryParameters")).Return([]*model.Trace{mockTrace}, nil).Once()
				queryString := "/api/traces?service=service&start=0&end=0&operation=operation&limit=200&minDuration=20ms"
				req, err := http.NewRequest("GET", "https://localhost:"+fmt.Sprintf("%d", ports.QueryHTTP)+queryString, nil)
				assert.Nil(t, err)
				req.Header.Add("Accept", "application/json")

				resp, err2 := client.Do(req)
				if err2 == nil {
					resp.Body.Close()
				}

				if test.expectClientError {
					require.Error(t, err2)
				} else {
					require.NoError(t, err2)
				}
			}
			server.Close()
			for i := 0; i < 10; i++ {
				if flagsSvc.HC().Get() == healthcheck.Unavailable {
					break
				}
				time.Sleep(1 * time.Millisecond)
			}
			assert.Equal(t, healthcheck.Unavailable, flagsSvc.HC().Get())

		})
	}
}

func newGRPCClientWithTLS(t *testing.T, addr string, creds credentials.TransportCredentials) *grpcClient {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	var conn *grpc.ClientConn
	var err error

	if creds != nil {
		conn, err = grpc.DialContext(ctx, addr, grpc.WithTransportCredentials(creds))
	} else {
		conn, err = grpc.DialContext(ctx, addr, grpc.WithInsecure())
	}

	require.NoError(t, err)
	return &grpcClient{
		QueryServiceClient: api_v2.NewQueryServiceClient(conn),
		conn:               conn,
	}
}

func TestServerGRPCTLS(t *testing.T) {
	testlen := len(testCases)

	tests := make([]struct {
		name              string
		HTTPTLS           tlscfg.Options
		GRPCTLS           tlscfg.Options
		clientTLS         tlscfg.Options
		expectError       bool
		expectClientError bool
		expectServerFail  bool
	}, testlen)
	copy(tests, testCases)
	tests[testlen-2].clientTLS = tlscfg.Options{Enabled: false}
	tests[testlen-2].name = "should pass with insecure GRPC Client and insecure GRPC server with secure HTTP Server"
	fmt.Println(tests[testlen-1])

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			serverOptions := &QueryOptions{TLSHTTP: test.HTTPTLS, TLSGRPC: test.GRPCTLS, HostPort: ports.PortToHostPort(ports.QueryHTTP), BearerTokenPropagation: true}
			flagsSvc := flags.NewService(ports.QueryAdminHTTP)
			flagsSvc.Logger = zap.NewNop()

			spanReader := &spanstoremocks.Reader{}
			dependencyReader := &depsmocks.Reader{}
			expectedServices := []string{"test"}
			spanReader.On("GetServices", mock.AnythingOfType("*context.valueCtx")).Return(expectedServices, nil)

			querySvc := querysvc.NewQueryService(spanReader, dependencyReader, querysvc.QueryServiceOptions{})
			server, err := NewServer(flagsSvc.Logger, querySvc,
				serverOptions,
				opentracing.NoopTracer{})
			assert.Nil(t, err)
			assert.NoError(t, server.Start())
			go func() {
				for s := range server.HealthCheckStatus() {
					flagsSvc.SetHealthCheckStatus(s)
				}
			}()

			var clientError error
			var client *grpcClient

			// time.Sleep(10 * time.Millisecond) // wait for server to start serving

			if serverOptions.TLSGRPC.Enabled {
				clientTLSCfg, err0 := test.clientTLS.Config()
				require.NoError(t, err0)
				creds := credentials.NewTLS(clientTLSCfg)
				client = newGRPCClientWithTLS(t, ports.PortToHostPort(ports.QueryHTTP), creds)

			} else {
				client = newGRPCClientWithTLS(t, ports.PortToHostPort(ports.QueryHTTP), nil)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			res, clientError := client.GetServices(ctx, &api_v2.GetServicesRequest{})

			if test.expectClientError {
				require.Error(t, clientError)
			} else {
				require.NoError(t, clientError)
				assert.Equal(t, expectedServices, res.Services)
			}
			if client != nil {
				require.Nil(t, client.conn.Close())
			}
			server.Close()
			for i := 0; i < 10; i++ {
				if flagsSvc.HC().Get() == healthcheck.Unavailable {
					break
				}
				time.Sleep(1 * time.Millisecond)
			}
			assert.Equal(t, healthcheck.Unavailable, flagsSvc.HC().Get())
		})
	}
}

func TestServer(t *testing.T) {
	flagsSvc := flags.NewService(ports.QueryAdminHTTP)
	flagsSvc.Logger = zap.NewNop()
	hostPort := ports.GetAddressFromCLIOptions(ports.QueryHTTP, "")

	spanReader := &spanstoremocks.Reader{}
	dependencyReader := &depsmocks.Reader{}
	expectedServices := []string{"test"}
	spanReader.On("GetServices", mock.AnythingOfType("*context.valueCtx")).Return(expectedServices, nil)

	querySvc := querysvc.NewQueryService(spanReader, dependencyReader, querysvc.QueryServiceOptions{})

	server, err := NewServer(flagsSvc.Logger, querySvc,
		&QueryOptions{HostPort: hostPort, BearerTokenPropagation: true},
		opentracing.NoopTracer{})
	assert.Nil(t, err)
	assert.NoError(t, server.Start())
	go func() {
		for s := range server.HealthCheckStatus() {
			flagsSvc.SetHealthCheckStatus(s)
		}
	}()

	client := newGRPCClient(t, hostPort)
	defer client.conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	res, err := client.GetServices(ctx, &api_v2.GetServicesRequest{})
	assert.NoError(t, err)
	assert.Equal(t, expectedServices, res.Services)

	server.Close()
	for i := 0; i < 10; i++ {
		if flagsSvc.HC().Get() == healthcheck.Unavailable {
			break
		}
		time.Sleep(1 * time.Millisecond)
	}
	assert.Equal(t, healthcheck.Unavailable, flagsSvc.HC().Get())
}

func TestServerGracefulExit(t *testing.T) {
	flagsSvc := flags.NewService(ports.QueryAdminHTTP)

	zapCore, logs := observer.New(zap.ErrorLevel)
	assert.Equal(t, 0, logs.Len(), "Expected initial ObservedLogs to have zero length.")

	flagsSvc.Logger = zap.New(zapCore)

	querySvc := &querysvc.QueryService{}
	tracer := opentracing.NoopTracer{}
	server, err := NewServer(flagsSvc.Logger, querySvc, &QueryOptions{HostPort: ports.PortToHostPort(ports.QueryAdminHTTP)}, tracer)
	assert.Nil(t, err)
	assert.NoError(t, server.Start())
	go func() {
		for s := range server.HealthCheckStatus() {
			flagsSvc.SetHealthCheckStatus(s)
		}
	}()

	// Wait for servers to come up before we can call .Close()
	// TODO Find a way to wait only as long as necessary. Unconditional sleep slows down the tests.
	time.Sleep(1 * time.Second)
	server.Close()

	for _, logEntry := range logs.All() {
		assert.True(t, logEntry.Level != zap.ErrorLevel,
			"Error log found on server exit: %v", logEntry)
	}
}

func TestServerHandlesPortZero(t *testing.T) {
	flagsSvc := flags.NewService(ports.QueryAdminHTTP)
	zapCore, logs := observer.New(zap.InfoLevel)
	flagsSvc.Logger = zap.New(zapCore)

	querySvc := &querysvc.QueryService{}
	tracer := opentracing.NoopTracer{}
	server, err := NewServer(flagsSvc.Logger, querySvc, &QueryOptions{HostPort: ":0"}, tracer)
	assert.Nil(t, err)
	assert.NoError(t, server.Start())
	server.Close()

	message := logs.FilterMessage("Query server started")
	assert.Equal(t, 1, message.Len(), "Expected query started log message.")

	onlyEntry := message.All()[0]
	port := onlyEntry.ContextMap()["port"]
	assert.Greater(t, port, int64(0))
}
