// Copyright (c) 2019,2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"fmt"
	"iter"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/config/confignet"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/config/configtls"
	"go.opentelemetry.io/collector/pdata/ptrace"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	traceapi "go.opentelemetry.io/otel/trace"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	"go.uber.org/zap/zaptest/observer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger-idl/proto-gen/api_v2"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/grpctest"
	depsmocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	tracestoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/v2/v1adapter"
	"github.com/jaegertracing/jaeger/internal/telemetry"
	"github.com/jaegertracing/jaeger/internal/tenancy"
)

var testCertKeyLocation = "../../../../../../internal/config/tlscfg/testdata"

func initTelSet(logger *zap.Logger, tracerProvider traceapi.TracerProvider) telemetry.Settings {
	telset := telemetry.NoopSettings()
	telset.Logger = logger
	telset.TracerProvider = tracerProvider
	return telset
}

func TestServerError(t *testing.T) {
	srv := &Server{
		queryOptions: &QueryOptions{
			HTTP: confighttp.ServerConfig{
				NetAddr: confignet.AddrConfig{
					Endpoint:  ":-1",
					Transport: confignet.TransportTypeTCP,
				},
			},
		},
	}
	require.Error(t, srv.Start(context.Background()))
}

func TestCreateTLSServerSinglePortError(t *testing.T) {
	// When TLS is enabled, and the host-port of both servers are the same, this leads to error, as TLS-enabled server is required to run on dedicated port.
	tlsCfg := configtls.ServerConfig{
		ClientCAFile: testCertKeyLocation + "/example-CA-cert.pem",
		Config: configtls.Config{
			CertFile: testCertKeyLocation + "/example-server-cert.pem",
			KeyFile:  testCertKeyLocation + "/example-server-key.pem",
		},
	}
	telset := initTelSet(zaptest.NewLogger(t), nooptrace.NewTracerProvider())
	_, err := NewServer(context.Background(), &querysvc.QueryService{}, nil,
		&QueryOptions{
			HTTP: confighttp.ServerConfig{
				NetAddr: confignet.AddrConfig{
					Endpoint:  ":8080",
					Transport: confignet.TransportTypeTCP,
				},
				TLS: configoptional.Some(tlsCfg),
			},
			GRPC: configgrpc.ServerConfig{NetAddr: confignet.AddrConfig{Endpoint: ":8080", Transport: confignet.TransportTypeTCP}, TLS: configoptional.Some(tlsCfg)},
		},
		tenancy.NewManager(&tenancy.Options{}), telset)
	require.Error(t, err)
}

func TestCreateTLSGrpcServerError(t *testing.T) {
	tlsCfg := configtls.ServerConfig{
		ClientCAFile: "invalid/path",
		Config: configtls.Config{
			CertFile: "invalid/path",
			KeyFile:  "invalid/path",
		},
	}
	telset := initTelSet(zaptest.NewLogger(t), nooptrace.NewTracerProvider())
	_, err := NewServer(context.Background(), &querysvc.QueryService{}, nil,
		&QueryOptions{
			HTTP: confighttp.ServerConfig{
				NetAddr: confignet.AddrConfig{
					Endpoint:  ":8080",
					Transport: confignet.TransportTypeTCP,
				},
			},
			GRPC: configgrpc.ServerConfig{NetAddr: confignet.AddrConfig{Endpoint: ":8081", Transport: confignet.TransportTypeTCP}, TLS: configoptional.Some(tlsCfg)},
		},
		tenancy.NewManager(&tenancy.Options{}), telset)
	require.Error(t, err)
}

func TestStartTLSHttpServerError(t *testing.T) {
	tlsCfg := configtls.ServerConfig{
		ClientCAFile: "invalid/path",
		Config: configtls.Config{
			CertFile: "invalid/path",
			KeyFile:  "invalid/path",
		},
	}
	telset := initTelSet(zaptest.NewLogger(t), nooptrace.NewTracerProvider())
	s, err := NewServer(context.Background(), &querysvc.QueryService{}, nil,
		&QueryOptions{
			HTTP: confighttp.ServerConfig{
				NetAddr: confignet.AddrConfig{
					Endpoint:  ":8080",
					Transport: confignet.TransportTypeTCP,
				},
				TLS: configoptional.Some(tlsCfg),
			},
			GRPC: configgrpc.ServerConfig{NetAddr: confignet.AddrConfig{Endpoint: ":8081", Transport: confignet.TransportTypeTCP}},
		}, tenancy.NewManager(&tenancy.Options{}), telset)
	require.NoError(t, err)
	require.Error(t, s.Start(context.Background()))
	t.Cleanup(func() {
		require.NoError(t, s.Close())
	})
}

var testCases = []struct {
	name              string
	TLS               *configtls.ServerConfig
	HTTPTLSEnabled    bool
	GRPCTLSEnabled    bool
	clientTLS         configtls.ClientConfig
	expectClientError bool
	expectServerFail  bool
}{
	{
		// this is a cross test for the "dedicated ports" use case without TLS
		name:           "Should pass with insecure connection",
		HTTPTLSEnabled: false,
		GRPCTLSEnabled: false,
		clientTLS: configtls.ClientConfig{
			Insecure: true,
		},
		expectClientError: false,
		expectServerFail:  false,
	},
	{
		name:           "should fail with TLS client to untrusted TLS server",
		HTTPTLSEnabled: true,
		GRPCTLSEnabled: true,
		TLS: &configtls.ServerConfig{
			Config: configtls.Config{
				CertFile: testCertKeyLocation + "/example-server-cert.pem",
				KeyFile:  testCertKeyLocation + "/example-server-key.pem",
			},
		},
		clientTLS: configtls.ClientConfig{
			Insecure:   true,
			ServerName: "example.com",
		},
		expectClientError: true,
		expectServerFail:  false,
	},
	{
		name:           "should fail with TLS client to trusted TLS server with incorrect hostname",
		HTTPTLSEnabled: true,
		GRPCTLSEnabled: true,
		TLS: &configtls.ServerConfig{
			Config: configtls.Config{
				CertFile: testCertKeyLocation + "/example-server-cert.pem",
				KeyFile:  testCertKeyLocation + "/example-server-key.pem",
			},
		},
		clientTLS: configtls.ClientConfig{
			Insecure:   false,
			ServerName: "nonEmpty",
			Config: configtls.Config{
				CAFile: testCertKeyLocation + "/example-CA-cert.pem",
			},
		},
		expectClientError: true,
		expectServerFail:  false,
	},
	{
		name:           "should pass with TLS client to trusted TLS server with correct hostname",
		HTTPTLSEnabled: true,
		GRPCTLSEnabled: true,
		TLS: &configtls.ServerConfig{
			Config: configtls.Config{
				CertFile: testCertKeyLocation + "/example-server-cert.pem",
				KeyFile:  testCertKeyLocation + "/example-server-key.pem",
			},
		},
		clientTLS: configtls.ClientConfig{
			Insecure:   false,
			ServerName: "example.com",
			Config: configtls.Config{
				CAFile: testCertKeyLocation + "/example-CA-cert.pem",
			},
		},
		expectClientError: false,
		expectServerFail:  false,
	},
	{
		name:           "should fail with TLS client without cert to trusted TLS server requiring cert",
		HTTPTLSEnabled: true,
		GRPCTLSEnabled: true,
		TLS: &configtls.ServerConfig{
			ClientCAFile: testCertKeyLocation + "/example-CA-cert.pem",
			Config: configtls.Config{
				CertFile: testCertKeyLocation + "/example-server-cert.pem",
				KeyFile:  testCertKeyLocation + "/example-server-key.pem",
			},
		},
		clientTLS: configtls.ClientConfig{
			Insecure:   false,
			ServerName: "example.com",
			Config: configtls.Config{
				CAFile: testCertKeyLocation + "/example-CA-cert.pem",
			},
		},
		expectServerFail:  false,
		expectClientError: true,
	},
	{
		name:           "should pass with TLS client with cert to trusted TLS server requiring cert",
		HTTPTLSEnabled: true,
		GRPCTLSEnabled: true,
		TLS: &configtls.ServerConfig{
			ClientCAFile: testCertKeyLocation + "/example-CA-cert.pem",
			Config: configtls.Config{
				CertFile: testCertKeyLocation + "/example-server-cert.pem",
				KeyFile:  testCertKeyLocation + "/example-server-key.pem",
			},
		},
		clientTLS: configtls.ClientConfig{
			Insecure:   false,
			ServerName: "example.com",
			Config: configtls.Config{
				CAFile:   testCertKeyLocation + "/example-CA-cert.pem",
				CertFile: testCertKeyLocation + "/example-client-cert.pem",
				KeyFile:  testCertKeyLocation + "/example-client-key.pem",
			},
		},
		expectServerFail:  false,
		expectClientError: false,
	},
	{
		name:           "should fail with TLS client without cert to trusted TLS server requiring cert from a different CA",
		HTTPTLSEnabled: true,
		GRPCTLSEnabled: true,
		TLS: &configtls.ServerConfig{
			ClientCAFile: testCertKeyLocation + "/wrong-CA-cert.pem", // NB: wrong CA
			Config: configtls.Config{
				CertFile: testCertKeyLocation + "/example-server-cert.pem",
				KeyFile:  testCertKeyLocation + "/example-server-key.pem",
			},
		},

		clientTLS: configtls.ClientConfig{
			Insecure:   false,
			ServerName: "example.com",
			Config: configtls.Config{
				CAFile:   testCertKeyLocation + "/example-CA-cert.pem",
				CertFile: testCertKeyLocation + "/example-client-cert.pem",
				KeyFile:  testCertKeyLocation + "/example-client-key.pem",
			},
		},
		expectServerFail:  false,
		expectClientError: true,
	},
	{
		name:           "should pass with TLS client with cert to trusted TLS HTTP server requiring cert and insecure GRPC server",
		HTTPTLSEnabled: true,
		GRPCTLSEnabled: false,
		TLS: &configtls.ServerConfig{
			ClientCAFile: testCertKeyLocation + "/example-CA-cert.pem",
			Config: configtls.Config{
				CertFile: testCertKeyLocation + "/example-server-cert.pem",
				KeyFile:  testCertKeyLocation + "/example-server-key.pem",
			},
		},
		clientTLS: configtls.ClientConfig{
			Insecure:   false,
			ServerName: "example.com",
			Config: configtls.Config{
				CAFile:   testCertKeyLocation + "/example-CA-cert.pem",
				CertFile: testCertKeyLocation + "/example-client-cert.pem",
				KeyFile:  testCertKeyLocation + "/example-client-key.pem",
			},
		},
		expectServerFail:  false,
		expectClientError: false,
	},
	{
		name:           "should pass with TLS client with cert to trusted GRPC TLS server requiring cert and insecure HTTP server",
		HTTPTLSEnabled: false,
		GRPCTLSEnabled: true,
		TLS: &configtls.ServerConfig{
			ClientCAFile: testCertKeyLocation + "/example-CA-cert.pem",
			Config: configtls.Config{
				CertFile: testCertKeyLocation + "/example-server-cert.pem",
				KeyFile:  testCertKeyLocation + "/example-server-key.pem",
			},
		},
		clientTLS: configtls.ClientConfig{
			Insecure:   false,
			ServerName: "example.com",
			Config: configtls.Config{
				CAFile:   testCertKeyLocation + "/example-CA-cert.pem",
				CertFile: testCertKeyLocation + "/example-client-cert.pem",
				KeyFile:  testCertKeyLocation + "/example-client-key.pem",
			},
		},
		expectServerFail:  false,
		expectClientError: false,
	},
}

type fakeQueryService struct {
	qs               *querysvc.QueryService
	traceReader      *tracestoremocks.Reader
	dependencyReader *depsmocks.Reader
	expectedServices []string
}

func makeQuerySvc() *fakeQueryService {
	traceReader := &tracestoremocks.Reader{}
	dependencyReader := &depsmocks.Reader{}
	expectedServices := []string{"test"}
	traceReader.On("GetServices", mock.Anything).Return(expectedServices, nil)
	qs := querysvc.NewQueryService(traceReader, dependencyReader, querysvc.QueryServiceOptions{})
	return &fakeQueryService{
		qs:               qs,
		traceReader:      traceReader,
		dependencyReader: dependencyReader,
		expectedServices: expectedServices,
	}
}

func makeMockPTrace() ptrace.Traces {
	trace := ptrace.NewTraces()
	resources := trace.ResourceSpans().AppendEmpty()
	resources.Resource().Attributes().PutStr("service.name", "service")
	scopes := resources.ScopeSpans().AppendEmpty()

	span1 := scopes.Spans().AppendEmpty()
	span1.SetTraceID(v1adapter.FromV1TraceID(model.NewTraceID(0, 0x123456abc)))
	span1.SetSpanID(v1adapter.FromV1SpanID(model.NewSpanID(1)))

	span2 := scopes.Spans().AppendEmpty()
	span2.SetTraceID(v1adapter.FromV1TraceID(model.NewTraceID(0, 0x123456abc)))
	span2.SetSpanID(v1adapter.FromV1SpanID(model.NewSpanID(2)))

	return trace
}

func optionalFromPtr[T any](ptr *T) configoptional.Optional[T] {
	if ptr == nil {
		return configoptional.None[T]()
	}
	return configoptional.Some(*ptr)
}

func TestServerHTTPTLS(t *testing.T) {
	testlen := len(testCases)

	tests := make([]struct {
		name              string
		TLS               *configtls.ServerConfig
		HTTPTLSEnabled    bool
		GRPCTLSEnabled    bool
		clientTLS         configtls.ClientConfig
		expectClientError bool
		expectServerFail  bool
	}, testlen)
	copy(tests, testCases)

	tests[testlen-1].clientTLS = configtls.ClientConfig{Insecure: true}
	tests[testlen-1].name = "Should pass with insecure HTTP Client and insecure HTTP server with secure GRPC Server"
	tests[testlen-1].TLS = nil

	var disabledTLSCfg *configtls.ServerConfig
	enabledTLSCfg := &configtls.ServerConfig{
		ClientCAFile: testCertKeyLocation + "/example-CA-cert.pem",
		Config: configtls.Config{
			CertFile: testCertKeyLocation + "/example-server-cert.pem",
			KeyFile:  testCertKeyLocation + "/example-server-key.pem",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tlsGrpc := disabledTLSCfg
			if test.GRPCTLSEnabled {
				tlsGrpc = enabledTLSCfg
			}
			serverOptions := &QueryOptions{
				BearerTokenPropagation: true,
				HTTP: confighttp.ServerConfig{
					NetAddr: confignet.AddrConfig{
						Endpoint:  ":0",
						Transport: confignet.TransportTypeTCP,
					},
					TLS: optionalFromPtr(test.TLS),
				},
				GRPC: configgrpc.ServerConfig{
					NetAddr: confignet.AddrConfig{
						Endpoint:  ":0",
						Transport: confignet.TransportTypeTCP,
					},
					TLS: optionalFromPtr(tlsGrpc),
				},
			}
			logger := zaptest.NewLogger(t)
			telset := initTelSet(logger, nooptrace.NewTracerProvider())
			querySvc := makeQuerySvc()
			server, err := NewServer(context.Background(), querySvc.qs,
				nil, serverOptions, tenancy.NewManager(&tenancy.Options{}),
				telset)
			require.NoError(t, err)
			require.NoError(t, server.Start(context.Background()))
			t.Cleanup(func() {
				require.NoError(t, server.Close())
			})

			if test.HTTPTLSEnabled {
				clientTLSCfg, err := test.clientTLS.LoadTLSConfig(context.Background())
				require.NoError(t, err)
				client := &http.Client{
					Transport: &http.Transport{
						TLSClientConfig: clientTLSCfg,
					},
				}
				querySvc.traceReader.On("FindTraces", mock.Anything, mock.Anything).
					Return(iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
						yield([]ptrace.Traces{makeMockPTrace()}, nil)
					})).Once()
				queryString := "/api/traces?service=service&start=0&end=0&operation=operation&limit=200&minDuration=20ms"
				req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("https://%s/%s", server.HTTPAddr(), queryString), http.NoBody)
				require.NoError(t, err)
				req.Header.Add("Accept", "application/json")

				resp, err := client.Do(req)
				t.Cleanup(func() {
					if err == nil {
						resp.Body.Close()
					}
				})

				if test.expectClientError {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
					require.Equal(t, http.StatusOK, resp.StatusCode)
				}
			}
		})
	}
}

func newGRPCClientWithTLS(t *testing.T, addr string, creds credentials.TransportCredentials) *grpcClient {
	var conn *grpc.ClientConn
	var err error

	if creds != nil {
		conn, err = grpc.NewClient(addr, grpc.WithTransportCredentials(creds))
	} else {
		conn, err = grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
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
		TLS               *configtls.ServerConfig
		HTTPTLSEnabled    bool
		GRPCTLSEnabled    bool
		clientTLS         configtls.ClientConfig
		expectClientError bool
		expectServerFail  bool
	}, testlen)
	copy(tests, testCases)
	tests[testlen-2].clientTLS = configtls.ClientConfig{Insecure: false}
	tests[testlen-2].name = "should pass with insecure GRPC Client and insecure GRPC server with secure HTTP Server"
	tests[testlen-2].TLS = nil

	var disabledTLSCfg *configtls.ServerConfig
	enabledTLSCfg := &configtls.ServerConfig{
		ClientCAFile: testCertKeyLocation + "/example-CA-cert.pem",
		Config: configtls.Config{
			CertFile: testCertKeyLocation + "/example-server-cert.pem",
			KeyFile:  testCertKeyLocation + "/example-server-key.pem",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tlsHttp := disabledTLSCfg
			if test.HTTPTLSEnabled {
				tlsHttp = enabledTLSCfg
			}
			serverOptions := &QueryOptions{
				BearerTokenPropagation: true,
				HTTP: confighttp.ServerConfig{
					NetAddr: confignet.AddrConfig{
						Endpoint:  ":0",
						Transport: confignet.TransportTypeTCP,
					},
					TLS: optionalFromPtr(tlsHttp),
				},
				GRPC: configgrpc.ServerConfig{
					NetAddr: confignet.AddrConfig{
						Endpoint:  ":0",
						Transport: confignet.TransportTypeTCP,
					},
					TLS: optionalFromPtr(test.TLS),
				},
			}
			logger := zaptest.NewLogger(t)

			querySvc := makeQuerySvc()
			telset := initTelSet(logger, nooptrace.NewTracerProvider())
			server, err := NewServer(context.Background(), querySvc.qs,
				nil, serverOptions, tenancy.NewManager(&tenancy.Options{}),
				telset)
			require.NoError(t, err)
			require.NoError(t, server.Start(context.Background()))
			t.Cleanup(func() {
				require.NoError(t, server.Close())
			})

			var client *grpcClient
			if serverOptions.GRPC.TLS.HasValue() {
				clientTLSCfg, err0 := test.clientTLS.LoadTLSConfig(context.Background())
				require.NoError(t, err0)
				creds := credentials.NewTLS(clientTLSCfg)
				client = newGRPCClientWithTLS(t, server.GRPCAddr(), creds)
			} else {
				client = newGRPCClientWithTLS(t, server.GRPCAddr(), nil)
			}
			t.Cleanup(func() {
				require.NoError(t, client.conn.Close())
			})

			// using generous timeout since grpc.NewClient no longer does a handshake.
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			logger.Info("calling client.GetServices()")
			res, clientError := client.GetServices(ctx, &api_v2.GetServicesRequest{})
			logger.Info("returned from GetServices()")

			if test.expectClientError {
				require.Error(t, clientError)
			} else {
				require.NoError(t, clientError)
				assert.Equal(t, querySvc.expectedServices, res.Services)
			}
		})
	}
}

func TestServerBadHostPort(t *testing.T) {
	telset := initTelSet(zaptest.NewLogger(t), nooptrace.NewTracerProvider())
	_, err := NewServer(context.Background(), &querysvc.QueryService{}, nil,
		&QueryOptions{
			BearerTokenPropagation: true,
			HTTP: confighttp.ServerConfig{
				NetAddr: confignet.AddrConfig{
					Endpoint:  "8080", // bad string, not :port
					Transport: confignet.TransportTypeTCP,
				},
			},
			GRPC: configgrpc.ServerConfig{
				NetAddr: confignet.AddrConfig{
					Endpoint:  "127.0.0.1:8081",
					Transport: confignet.TransportTypeTCP,
				},
			},
		},
		tenancy.NewManager(&tenancy.Options{}),
		telset)
	require.Error(t, err)

	_, err = NewServer(context.Background(), &querysvc.QueryService{}, nil,
		&QueryOptions{
			BearerTokenPropagation: true,
			HTTP: confighttp.ServerConfig{
				NetAddr: confignet.AddrConfig{
					Endpoint:  "127.0.0.1:8081",
					Transport: confignet.TransportTypeTCP,
				},
			},
			GRPC: configgrpc.ServerConfig{
				NetAddr: confignet.AddrConfig{
					Endpoint:  "9123", // bad string, not :port
					Transport: confignet.TransportTypeTCP,
				},
			},
		},
		tenancy.NewManager(&tenancy.Options{}),
		telset)

	require.Error(t, err)
}

func TestServerInUseHostPort(t *testing.T) {
	const availableHostPort = "127.0.0.1:0"
	conn, err := net.Listen("tcp", availableHostPort)
	require.NoError(t, err)
	defer func() { require.NoError(t, conn.Close()) }()
	telset := initTelSet(zaptest.NewLogger(t), nooptrace.NewTracerProvider())
	testCases := []struct {
		name         string
		httpHostPort string
		grpcHostPort string
	}{
		{"HTTP host port clash", conn.Addr().String(), availableHostPort},
		{"GRPC host port clash", availableHostPort, conn.Addr().String()},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server, err := NewServer(
				context.Background(),
				&querysvc.QueryService{},
				nil,
				&QueryOptions{
					BearerTokenPropagation: true,
					HTTP: confighttp.ServerConfig{
						NetAddr: confignet.AddrConfig{
							Endpoint:  tc.httpHostPort,
							Transport: confignet.TransportTypeTCP,
						},
					},
					GRPC: configgrpc.ServerConfig{
						NetAddr: confignet.AddrConfig{
							Endpoint:  tc.grpcHostPort,
							Transport: confignet.TransportTypeTCP,
						},
					},
				},
				tenancy.NewManager(&tenancy.Options{}),
				telset,
			)
			require.NoError(t, err)
			require.Error(t, server.Start(context.Background()))
			server.Close()
		})
	}
}

func TestServerGracefulExit(t *testing.T) {
	zapCore, logs := observer.New(zap.ErrorLevel)
	assert.Equal(t, 0, logs.Len(), "Expected initial ObservedLogs to have zero length.")

	logger := zap.New(zapCore)
	telset := initTelSet(logger, nooptrace.NewTracerProvider())
	traceReader := &tracestoremocks.Reader{}
	traceReader.On("GetServices", mock.Anything).Return([]string{"test"}, nil)
	qs := querysvc.NewQueryService(traceReader, &depsmocks.Reader{}, querysvc.QueryServiceOptions{})
	server, err := NewServer(context.Background(), qs, nil,
		&QueryOptions{
			HTTP: confighttp.ServerConfig{
				NetAddr: confignet.AddrConfig{
					Endpoint:  ":0",
					Transport: confignet.TransportTypeTCP,
				},
			},
			GRPC: configgrpc.ServerConfig{
				NetAddr: confignet.AddrConfig{
					Endpoint:  ":0",
					Transport: confignet.TransportTypeTCP,
				},
			},
		},
		tenancy.NewManager(&tenancy.Options{}), telset)
	require.NoError(t, err)
	require.NoError(t, server.Start(context.Background()))

	// Wait for servers to come up before we can call .Close()
	{
		client := newGRPCClient(t, server.GRPCAddr())
		t.Cleanup(func() {
			require.NoError(t, client.conn.Close())
		})
		// using generous timeout since grpc.NewClient no longer does a handshake.
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, err := client.GetServices(ctx, &api_v2.GetServicesRequest{})
		require.NoError(t, err)
	}

	server.Close()
	for _, logEntry := range logs.All() {
		assert.NotEqual(t, zap.ErrorLevel, logEntry.Level,
			"Error log found on server exit: %v", logEntry)
	}
}

func TestServerHandlesPortZero(t *testing.T) {
	zapCore, logs := observer.New(zap.InfoLevel)
	logger := zap.New(zapCore)

	v2QuerySvc := &querysvc.QueryService{}
	telset := initTelSet(logger, nooptrace.NewTracerProvider())
	server, err := NewServer(context.Background(), v2QuerySvc, nil,
		&QueryOptions{
			HTTP: confighttp.ServerConfig{
				NetAddr: confignet.AddrConfig{
					Endpoint:  ":0",
					Transport: confignet.TransportTypeTCP,
				},
			},
			GRPC: configgrpc.ServerConfig{
				NetAddr: confignet.AddrConfig{
					Endpoint:  ":0",
					Transport: confignet.TransportTypeTCP,
				},
			},
		},
		tenancy.NewManager(&tenancy.Options{}),
		telset)
	require.NoError(t, err)
	require.NoError(t, server.Start(context.Background()))
	defer server.Close()

	message := logs.FilterMessage("Query server started")
	assert.Equal(t, 1, message.Len(), "Expected 'Query server started' log message.")

	grpctest.ReflectionServiceValidator{
		HostPort: server.GRPCAddr(),
		ExpectedServices: []string{
			"jaeger.api_v2.QueryService",
			"jaeger.api_v3.QueryService",
			"grpc.health.v1.Health",
		},
	}.Execute(t)
}

func TestServerHTTPTenancy(t *testing.T) {
	testCases := []struct {
		name   string
		tenant string
		errMsg string
		status int
	}{
		{
			name: "no tenant",
			// no value for tenant header
			status: http.StatusUnauthorized,
		},
		{
			name:   "tenant",
			tenant: "acme",
			status: http.StatusOK,
		},
	}

	serverOptions := &QueryOptions{
		Tenancy: tenancy.Options{
			Enabled: true,
		}, HTTP: confighttp.ServerConfig{
			NetAddr: confignet.AddrConfig{
				Endpoint:  ":0",
				Transport: confignet.TransportTypeTCP,
			},
		},
		GRPC: configgrpc.ServerConfig{
			NetAddr: confignet.AddrConfig{
				Endpoint:  ":0",
				Transport: confignet.TransportTypeTCP,
			},
		},
	}
	tenancyMgr := tenancy.NewManager(&serverOptions.Tenancy)
	querySvc := makeQuerySvc()
	querySvc.traceReader.On("FindTraces", mock.Anything, mock.Anything).
		Return(iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
			yield([]ptrace.Traces{makeMockPTrace()}, nil)
		})).Once()
	telset := initTelSet(zaptest.NewLogger(t), nooptrace.NewTracerProvider())
	server, err := NewServer(context.Background(), querySvc.qs,
		nil, serverOptions, tenancyMgr, telset)
	require.NoError(t, err)
	require.NoError(t, server.Start(context.Background()))
	t.Cleanup(func() {
		require.NoError(t, server.Close())
	})

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			conn, clientError := net.DialTimeout("tcp", server.HTTPAddr(), 2*time.Second)
			require.NoError(t, clientError)

			queryString := "/api/traces?service=service&start=0&end=0&operation=operation&limit=200&minDuration=20ms"
			req, err := http.NewRequest(http.MethodGet, "http://"+server.HTTPAddr()+queryString, http.NoBody)
			if test.tenant != "" {
				req.Header.Add(tenancyMgr.Header, test.tenant)
			}
			require.NoError(t, err)
			req.Header.Add("Accept", "application/json")

			client := &http.Client{}
			resp, err2 := client.Do(req)
			if test.errMsg == "" {
				require.NoError(t, err2)
			} else {
				require.Error(t, err2)
				if err != nil {
					assert.Equal(t, test.errMsg, err2.Error())
				}
			}
			assert.Equal(t, test.status, resp.StatusCode)
			if err2 == nil {
				resp.Body.Close()
			}
			if conn != nil {
				require.NoError(t, conn.Close())
			}
		})
	}
}

func TestServerHTTP_TracesRequest(t *testing.T) {
	tests := []struct {
		name          string
		httpEndpoint  string
		grpcEndpoint  string
		queryString   string
		expectedTrace string
	}{
		{
			name:          "api v1/v2",
			httpEndpoint:  ":8080",
			grpcEndpoint:  ":8081",
			queryString:   "/api/traces/123456aBC",
			expectedTrace: "/api/traces/{traceID}",
		},
		{
			name:          "api v3",
			httpEndpoint:  ":8080",
			grpcEndpoint:  ":8081",
			queryString:   "/api/v3/traces/123456aBC",
			expectedTrace: "/api/v3/traces/{trace_id}",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			serverOptions := &QueryOptions{
				HTTP: confighttp.ServerConfig{
					NetAddr: confignet.AddrConfig{
						Endpoint:  ":0",
						Transport: confignet.TransportTypeTCP,
					},
				},
				GRPC: configgrpc.ServerConfig{
					NetAddr: confignet.AddrConfig{
						Endpoint:  ":0",
						Transport: confignet.TransportTypeTCP,
					},
				},
			}

			exporter := tracetest.NewInMemoryExporter()
			tracerProvider := tracesdk.NewTracerProvider(
				tracesdk.WithSyncer(exporter),
				tracesdk.WithSampler(tracesdk.AlwaysSample()),
			)
			t.Cleanup(func() {
				require.NoError(t, tracerProvider.Shutdown(context.Background()))
			})
			tenancyMgr := tenancy.NewManager(&serverOptions.Tenancy)
			querySvc := makeQuerySvc()
			querySvc.traceReader.On("GetTraces", mock.Anything, mock.MatchedBy(func(params []tracestore.GetTraceParams) bool {
				return len(params) == 1 && params[0].TraceID == v1adapter.FromV1TraceID(model.NewTraceID(0, 0x123456abc))
			})).Return(iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
				yield([]ptrace.Traces{makeMockPTrace()}, nil)
			})).Once()
			telset := initTelSet(zaptest.NewLogger(t), tracerProvider)

			server, err := NewServer(context.Background(), querySvc.qs,
				nil, serverOptions, tenancyMgr, telset)
			require.NoError(t, err)
			require.NoError(t, server.Start(context.Background()))
			t.Cleanup(func() {
				require.NoError(t, server.Close())
			})

			req, err := http.NewRequest(http.MethodGet, "http://"+server.HTTPAddr()+test.queryString, http.NoBody)
			require.NoError(t, err)

			client := &http.Client{}
			resp, err := client.Do(req)
			require.NoError(t, err)

			if test.expectedTrace != "" {
				assert.Len(t, exporter.GetSpans(), 1, "HTTP request was traced and span reported")
				assert.Equal(t, test.expectedTrace, exporter.GetSpans()[0].Name)
			} else {
				assert.Empty(t, exporter.GetSpans(), "HTTP request was not traced")
			}
			require.NoError(t, resp.Body.Close())
		})
	}
}

func TestServerAPINotFound(t *testing.T) {
	querySvc := makeQuerySvc()

	for _, basePath := range []string{"", "/jaeger"} {
		t.Run("basePath="+basePath, func(t *testing.T) {
			serverOptions := &QueryOptions{
				HTTP: confighttp.ServerConfig{
					NetAddr: confignet.AddrConfig{
						Endpoint:  ":0",
						Transport: confignet.TransportTypeTCP,
					},
				},
				GRPC: configgrpc.ServerConfig{
					NetAddr: confignet.AddrConfig{
						Endpoint:  ":0",
						Transport: confignet.TransportTypeTCP,
					},
				},
				BasePath: basePath,
			}
			tenancyMgr := tenancy.NewManager(&serverOptions.Tenancy)
			telset := initTelSet(zaptest.NewLogger(t), nooptrace.NewTracerProvider())

			server, err := NewServer(context.Background(), querySvc.qs, nil, serverOptions, tenancyMgr, telset)
			require.NoError(t, err)
			require.NoError(t, server.Start(context.Background()))
			t.Cleanup(func() {
				require.NoError(t, server.Close())
			})

			tests := []struct {
				name           string
				path           string
				expectedStatus int
			}{
				{
					name:           "existing API endpoint returns 200",
					path:           basePath + "/api/services",
					expectedStatus: http.StatusOK,
				},
				{
					name:           "non-existent API endpoint returns 404",
					path:           basePath + "/api/nonexistent",
					expectedStatus: http.StatusNotFound,
				},
				{
					name:           "root returns 200 (index.html)",
					path:           basePath + "/",
					expectedStatus: http.StatusOK,
				},
			}

			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					req, err := http.NewRequest(http.MethodGet, "http://"+server.HTTPAddr()+test.path, http.NoBody)
					require.NoError(t, err)

					client := &http.Client{}
					resp, err := client.Do(req)
					require.NoError(t, err)
					defer resp.Body.Close()

					assert.Equal(t, test.expectedStatus, resp.StatusCode)
				})
			}
		})
	}
}
