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
	"github.com/jaegertracing/jaeger/pkg/healthcheck"
	"github.com/jaegertracing/jaeger/pkg/telemetry"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
	"github.com/jaegertracing/jaeger/plugin/storage"
	"github.com/jaegertracing/jaeger/ports"
	"github.com/jaegertracing/jaeger/proto-gen/storage_v1"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	depStoreMocks "github.com/jaegertracing/jaeger/storage/dependencystore/mocks"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	spanStoreMocks "github.com/jaegertracing/jaeger/storage/spanstore/mocks"
)

var testCertKeyLocation = "../../../pkg/config/tlscfg/testdata"

func defaultFactoryCfg() storage.FactoryConfig {
	return storage.FactoryConfig{
		SpanWriterTypes:         []string{"blackhole"},
		SpanReaderType:          "blackhole",
		DependenciesStorageType: "blackhole",
		DownsamplingRatio:       1.0,
		DownsamplingHashSalt:    "",
	}
}

func TestNewServer_CreateStorageErrors(t *testing.T) {
	tests := []struct {
		name          string
		modifyFactory func(factory *storage.FactoryConfig)
		expectedError string
	}{
		{
			name: "no span reader",
			modifyFactory: func(factory *storage.FactoryConfig) {
				factory.SpanReaderType = "a"
			},
			expectedError: "no a backend registered",
		},
		{
			name: "no span writer",
			modifyFactory: func(factory *storage.FactoryConfig) {
				factory.SpanWriterTypes = []string{"b"}
			},
			expectedError: "no b backend registered",
		},
		{
			name: "no dependency reader",
			modifyFactory: func(factory *storage.FactoryConfig) {
				factory.DependenciesStorageType = "c"
			},
			expectedError: "no c backend registered",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			factoryCfg := defaultFactoryCfg()

			factory, err := storage.NewFactory(factoryCfg)
			require.NoError(t, err)

			test.modifyFactory(&factory.FactoryConfig)

			_, err = NewServer(
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

			require.ErrorContains(t, err, test.expectedError)
		})
	}
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
	reader    *spanStoreMocks.Reader
	writer    *spanStoreMocks.Writer
	depReader *depStoreMocks.Reader
}

func (f *fakeFactory) CreateSpanReader() (spanstore.Reader, error) {
	return f.reader, nil
}

func (f *fakeFactory) CreateSpanWriter() (spanstore.Writer, error) {
	return f.writer, nil
}

func (f *fakeFactory) CreateDependencyReader() (dependencystore.Reader, error) {
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

	f, err := storage.NewFactory(defaultFactoryCfg())
	require.NoError(t, err)

	_, err = NewServer(
		&Options{
			ServerConfig: configgrpc.ServerConfig{
				NetAddr: confignet.AddrConfig{
					Endpoint: ":8081",
				},
				TLSSetting: tlsCfg,
			},
		},
		f,
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

	h, err := createGRPCHandler(f, zap.NewNop())
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

	err = h.GetArchiveTrace(nil, nil)
	require.ErrorContains(t, err, "not implemented")

	_, err = h.WriteArchiveSpan(context.Background(), nil)
	require.ErrorContains(t, err, "not implemented")

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
	f, err := storage.NewFactory(defaultFactoryCfg())
	require.NoError(t, err)
	telset := telemetry.Settings{
		Logger:       flagsSvc.Logger,
		ReportStatus: telemetry.HCAdapter(flagsSvc.HC()),
	}
	server, err := NewServer(
		&Options{ServerConfig: configgrpc.ServerConfig{
			NetAddr: confignet.AddrConfig{Endpoint: ":0"},
		}},
		f,
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
			"jaeger.storage.v1.ArchiveSpanReaderPlugin",
			"jaeger.storage.v1.ArchiveSpanWriterPlugin",
			"jaeger.storage.v1.StreamingSpanWriterPlugin",
			"grpc.health.v1.Health",
		},
	}.Execute(t)
}
