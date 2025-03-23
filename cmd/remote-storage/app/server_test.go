// Copyright (c) 2022 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/confignet"
	"go.opentelemetry.io/collector/config/configtls"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/jaegertracing/jaeger/cmd/internal/flags"
	"github.com/jaegertracing/jaeger/internal/grpctest"
	"github.com/jaegertracing/jaeger/internal/healthcheck"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/dependencystore"
	depStoreMocks "github.com/jaegertracing/jaeger/internal/storage/v1/api/dependencystore/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
	spanStoreMocks "github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore/mocks"
	"github.com/jaegertracing/jaeger/internal/telemetry"
	"github.com/jaegertracing/jaeger/internal/tenancy"
	"github.com/jaegertracing/jaeger/ports"
	"github.com/jaegertracing/jaeger/proto-gen/storage_v1"
)

var testCertKeyLocation = "../../../internal/config/tlscfg/testdata"

func TestNewServer_CreateStorageErrors(t *testing.T) {
	createServer := func(factory *fakeFactory) (*Server, error) {
		return NewServer(
			&Options{
				ServerConfig: configgrpc.ServerConfig{
					NetAddr: confignet.AddrConfig{
						Endpoint: ":0",
					},
				},
			},
			factory,
			tenancy.NewManager(&tenancy.Options{}),
			telemetry.NoopSettings(),
		)
	}

	factory := &fakeFactory{readerErr: errors.New("no reader")}
	_, err := createServer(factory)
	require.ErrorContains(t, err, "no reader")

	factory = &fakeFactory{writerErr: errors.New("no writer")}
	_, err = createServer(factory)
	require.ErrorContains(t, err, "no writer")

	factory = &fakeFactory{depReaderErr: errors.New("no deps")}
	_, err = createServer(factory)
	require.ErrorContains(t, err, "no deps")

	factory = &fakeFactory{}
	s, err := createServer(factory)
	require.NoError(t, err)
	require.NoError(t, s.Start())
	validateGRPCServer(t, s.grpcConn.Addr().String())
	require.NoError(t, s.grpcConn.Close())
}

func TestServerStart_BadPortErrors(t *testing.T) {
	srv := &Server{
		opts: &Options{
			ServerConfig: configgrpc.ServerConfig{
				NetAddr: confignet.AddrConfig{
					Endpoint: ":-1",
				},
			},
		},
	}
	require.Error(t, srv.Start())
}

type fakeFactory struct {
	reader    spanstore.Reader
	writer    spanstore.Writer
	depReader dependencystore.Reader

	readerErr    error
	writerErr    error
	depReaderErr error
}

func (f *fakeFactory) CreateSpanReader() (spanstore.Reader, error) {
	if f.readerErr != nil {
		return nil, f.readerErr
	}
	return f.reader, nil
}

func (f *fakeFactory) CreateSpanWriter() (spanstore.Writer, error) {
	if f.writerErr != nil {
		return nil, f.writerErr
	}
	return f.writer, nil
}

func (f *fakeFactory) CreateDependencyReader() (dependencystore.Reader, error) {
	if f.depReaderErr != nil {
		return nil, f.depReaderErr
	}
	return f.depReader, nil
}

func (*fakeFactory) InitArchiveStorage(*zap.Logger) (spanstore.Reader, spanstore.Writer) {
	return nil, nil
}

func TestNewServer_TLSConfigError(t *testing.T) {
	tlsCfg := &configtls.ServerConfig{
		ClientCAFile: "invalid/path",
		Config: configtls.Config{
			CertFile: "invalid/path",
			KeyFile:  "invalid/path",
		},
	}
	telset := telemetry.Settings{
		Logger:       zap.NewNop(),
		ReportStatus: telemetry.HCAdapter(healthcheck.New()),
	}

	_, err := NewServer(
		&Options{
			ServerConfig: configgrpc.ServerConfig{
				NetAddr: confignet.AddrConfig{
					Endpoint: ":8081",
				},
				TLSSetting: tlsCfg,
			},
		},
		&fakeFactory{},
		tenancy.NewManager(&tenancy.Options{}),
		telset,
	)
	assert.ErrorContains(t, err, "failed to load TLS config")
}

func TestCreateGRPCHandler(t *testing.T) {
	reader := new(spanStoreMocks.Reader)
	writer := new(spanStoreMocks.Writer)
	depReader := new(depStoreMocks.Reader)

	f := &fakeFactory{
		reader:    reader,
		writer:    writer,
		depReader: depReader,
	}

	h, err := createGRPCHandler(f)
	require.NoError(t, err)

	writer.On("WriteSpan", mock.Anything, mock.Anything).Return(errors.New("writer error"))
	_, err = h.WriteSpan(context.Background(), &storage_v1.WriteSpanRequest{})
	require.ErrorContains(t, err, "writer error")

	depReader.On(
		"GetDependencies",
		mock.Anything, // context
		mock.Anything, // time
		mock.Anything, // lookback
	).Return(nil, errors.New("deps error"))
	_, err = h.GetDependencies(context.Background(), &storage_v1.GetDependenciesRequest{})
	require.ErrorContains(t, err, "deps error")

	err = h.WriteSpanStream(nil)
	assert.ErrorContains(t, err, "not implemented")
}

var testCases = []struct {
	name              string
	TLS               *configtls.ServerConfig
	clientTLS         *configtls.ClientConfig
	expectError       bool
	expectClientError bool
	expectServerFail  bool
}{
	{
		name: "should pass with insecure connection",
		TLS:  nil,
		clientTLS: &configtls.ClientConfig{
			Insecure: true,
		},
		expectError:       false,
		expectClientError: false,
		expectServerFail:  false,
	},
	{
		name: "should fail with TLS client to untrusted TLS server",
		TLS: &configtls.ServerConfig{
			Config: configtls.Config{
				CertFile: testCertKeyLocation + "/example-server-cert.pem",
				KeyFile:  testCertKeyLocation + "/example-server-key.pem",
			},
		},
		clientTLS: &configtls.ClientConfig{
			ServerName: "example.com",
		},
		expectError:       true,
		expectClientError: true,
		expectServerFail:  false,
	},
	{
		name: "should fail with TLS client to trusted TLS server with incorrect hostname",
		TLS: &configtls.ServerConfig{
			Config: configtls.Config{
				CertFile: testCertKeyLocation + "/example-server-cert.pem",
				KeyFile:  testCertKeyLocation + "/example-server-key.pem",
			},
		},
		clientTLS: &configtls.ClientConfig{
			Config: configtls.Config{
				CAFile: testCertKeyLocation + "/example-CA-cert.pem",
			},
			ServerName: "nonEmpty",
		},
		expectError:       true,
		expectClientError: true,
		expectServerFail:  false,
	},
	{
		name: "should pass with TLS client to trusted TLS server with correct hostname",
		TLS: &configtls.ServerConfig{
			Config: configtls.Config{
				CertFile: testCertKeyLocation + "/example-server-cert.pem",
				KeyFile:  testCertKeyLocation + "/example-server-key.pem",
			},
		},
		clientTLS: &configtls.ClientConfig{
			Config: configtls.Config{
				CAFile: testCertKeyLocation + "/example-CA-cert.pem",
			},
			ServerName: "example.com",
		},
		expectError:       false,
		expectClientError: false,
		expectServerFail:  false,
	},
	{
		name: "should fail with TLS client without cert to trusted TLS server requiring cert",
		TLS: &configtls.ServerConfig{
			Config: configtls.Config{
				CertFile: testCertKeyLocation + "/example-server-cert.pem",
				KeyFile:  testCertKeyLocation + "/example-server-key.pem",
			},
			ClientCAFile: testCertKeyLocation + "/example-CA-cert.pem",
		},
		clientTLS: &configtls.ClientConfig{
			Config: configtls.Config{
				CAFile: testCertKeyLocation + "/example-CA-cert.pem",
			},
			ServerName: "example.com",
		},
		expectError:       false,
		expectServerFail:  false,
		expectClientError: true,
	},
	{
		name: "should pass with TLS client with cert to trusted TLS server requiring cert",
		TLS: &configtls.ServerConfig{
			Config: configtls.Config{
				CertFile: testCertKeyLocation + "/example-server-cert.pem",
				KeyFile:  testCertKeyLocation + "/example-server-key.pem",
			},
			ClientCAFile: testCertKeyLocation + "/example-CA-cert.pem",
		},
		clientTLS: &configtls.ClientConfig{
			Config: configtls.Config{
				CAFile:   testCertKeyLocation + "/example-CA-cert.pem",
				CertFile: testCertKeyLocation + "/example-client-cert.pem",
				KeyFile:  testCertKeyLocation + "/example-client-key.pem",
			},
			ServerName: "example.com",
		},
		expectError:       false,
		expectServerFail:  false,
		expectClientError: false,
	},
	{
		name: "should fail with TLS client without cert to trusted TLS server requiring cert from a different CA",
		TLS: &configtls.ServerConfig{
			Config: configtls.Config{
				CertFile: testCertKeyLocation + "/example-server-cert.pem",
				KeyFile:  testCertKeyLocation + "/example-server-key.pem",
			},
			ClientCAFile: testCertKeyLocation + "/wrong-CA-cert.pem",
		},
		clientTLS: &configtls.ClientConfig{
			Config: configtls.Config{
				CAFile:   testCertKeyLocation + "/example-CA-cert.pem",
				CertFile: testCertKeyLocation + "/example-client-cert.pem",
				KeyFile:  testCertKeyLocation + "/example-client-key.pem",
			},
			ServerName: "example.com",
		},
		expectError:       false,
		expectServerFail:  false,
		expectClientError: true,
	},
}

type grpcClient struct {
	storage_v1.SpanReaderPluginClient

	conn *grpc.ClientConn
}

func newGRPCClient(t *testing.T, addr string, creds credentials.TransportCredentials, tm *tenancy.Manager) *grpcClient {
	dialOpts := []grpc.DialOption{
		grpc.WithUnaryInterceptor(tenancy.NewClientUnaryInterceptor(tm)),
	}
	if creds != nil {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(creds))
	} else {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	conn, err := grpc.NewClient(addr, dialOpts...)
	require.NoError(t, err)

	return &grpcClient{
		SpanReaderPluginClient: storage_v1.NewSpanReaderPluginClient(conn),
		conn:                   conn,
	}
}

func TestServerGRPCTLS(t *testing.T) {
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			serverOptions := &Options{
				ServerConfig: configgrpc.ServerConfig{
					NetAddr: confignet.AddrConfig{
						Endpoint: ":0",
					},
					TLSSetting: test.TLS,
				},
			}
			flagsSvc := flags.NewService(ports.QueryAdminHTTP)
			flagsSvc.Logger = zap.NewNop()

			reader := new(spanStoreMocks.Reader)
			f := &fakeFactory{
				reader: reader,
			}
			expectedServices := []string{"test"}
			reader.On("GetServices", mock.AnythingOfType("*context.valueCtx")).Return(expectedServices, nil)

			tm := tenancy.NewManager(&tenancy.Options{Enabled: true})
			telset := telemetry.Settings{
				Logger:       flagsSvc.Logger,
				ReportStatus: telemetry.HCAdapter(flagsSvc.HC()),
			}
			server, err := NewServer(
				serverOptions,
				f,
				tm,
				telset,
			)
			require.NoError(t, err)
			require.NoError(t, server.Start())

			var clientError error
			var client *grpcClient

			if serverOptions.TLSSetting != nil {
				clientTLSCfg, err0 := test.clientTLS.LoadTLSConfig(context.Background())
				require.NoError(t, err0)
				creds := credentials.NewTLS(clientTLSCfg)
				client = newGRPCClient(t, server.grpcConn.Addr().String(), creds, tm)
			} else {
				client = newGRPCClient(t, server.grpcConn.Addr().String(), nil, tm)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			ctx = tenancy.WithTenant(ctx, "foo")
			res, clientError := client.GetServices(ctx, &storage_v1.GetServicesRequest{})

			if test.expectClientError {
				require.Error(t, clientError)
			} else {
				require.NoError(t, clientError)
				assert.Equal(t, expectedServices, res.Services)
			}
			require.NoError(t, client.conn.Close())
			server.Close()
			assert.Equal(t, healthcheck.Unavailable, flagsSvc.HC().Get())
		})
	}
}

func TestServerHandlesPortZero(t *testing.T) {
	flagsSvc := flags.NewService(ports.QueryAdminHTTP)
	zapCore, logs := observer.New(zap.InfoLevel)
	flagsSvc.Logger = zap.New(zapCore)
	telset := telemetry.Settings{
		Logger:       flagsSvc.Logger,
		ReportStatus: telemetry.HCAdapter(flagsSvc.HC()),
	}
	server, err := NewServer(
		&Options{ServerConfig: configgrpc.ServerConfig{
			NetAddr: confignet.AddrConfig{Endpoint: ":0"},
		}},
		&fakeFactory{},
		tenancy.NewManager(&tenancy.Options{}),
		telset,
	)
	require.NoError(t, err)

	require.NoError(t, server.Start())

	const line = "Starting GRPC server"
	message := logs.FilterMessage(line)
	require.Equal(t, 1, message.Len(), "Expected '%s' log message, actual logs: %+v", line, logs)

	onlyEntry := message.All()[0]
	hostPort := onlyEntry.ContextMap()["addr"].(string)
	validateGRPCServer(t, hostPort)

	server.Close()

	assert.Equal(t, healthcheck.Unavailable, flagsSvc.HC().Get())
}

func validateGRPCServer(t *testing.T, hostPort string) {
	grpctest.ReflectionServiceValidator{
		HostPort: hostPort,
		ExpectedServices: []string{
			"jaeger.storage.v1.SpanReaderPlugin",
			"jaeger.storage.v1.SpanWriterPlugin",
			"jaeger.storage.v1.DependenciesReaderPlugin",
			"jaeger.storage.v1.PluginCapabilities",
			"jaeger.storage.v1.StreamingSpanWriterPlugin",
			"grpc.health.v1.Health",
		},
	}.Execute(t)
}
