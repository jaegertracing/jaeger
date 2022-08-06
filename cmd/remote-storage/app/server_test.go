// Copyright (c) 2022 The Jaeger Authors.
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
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/jaegertracing/jaeger/cmd/flags"
	"github.com/jaegertracing/jaeger/internal/grpctest"
	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
	"github.com/jaegertracing/jaeger/pkg/healthcheck"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
	"github.com/jaegertracing/jaeger/ports"
	"github.com/jaegertracing/jaeger/proto-gen/storage_v1"
	depStoreMocks "github.com/jaegertracing/jaeger/storage/dependencystore/mocks"
	factoryMocks "github.com/jaegertracing/jaeger/storage/mocks"
	spanStoreMocks "github.com/jaegertracing/jaeger/storage/spanstore/mocks"
)

var testCertKeyLocation = "../../../pkg/config/tlscfg/testdata"

func TestNewServer_CreateStorageErrors(t *testing.T) {
	factory := new(factoryMocks.Factory)
	factory.On("CreateSpanReader").Return(nil, errors.New("no reader")).Once()
	factory.On("CreateSpanReader").Return(nil, nil)
	factory.On("CreateSpanWriter").Return(nil, errors.New("no writer")).Once()
	factory.On("CreateSpanWriter").Return(nil, nil)
	factory.On("CreateDependencyReader").Return(nil, errors.New("no deps")).Once()
	factory.On("CreateDependencyReader").Return(nil, nil)

	f := func() (*Server, error) {
		return NewServer(
			&Options{GRPCHostPort: ":0"},
			factory,
			tenancy.NewManager(&tenancy.Options{}),
			zap.NewNop(),
		)
	}
	_, err := f()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no reader")

	_, err = f()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no writer")

	_, err = f()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no deps")

	s, err := f()
	require.NoError(t, err)
	err = s.Start()
	require.NoError(t, err)
	validateGRPCServer(t, s.grpcConn.Addr().String(), s.grpcServer)

	s.grpcConn.Close() // causes logged error
	<-s.HealthCheckStatus()
}

func TestServerStart_BadPortErrors(t *testing.T) {
	srv := &Server{
		opts: &Options{
			GRPCHostPort: ":-1",
		},
	}
	assert.Error(t, srv.Start())
}

type storageMocks struct {
	factory   *factoryMocks.Factory
	reader    *spanStoreMocks.Reader
	writer    *spanStoreMocks.Writer
	depReader *depStoreMocks.Reader
}

func newStorageMocks() *storageMocks {
	reader := new(spanStoreMocks.Reader)
	writer := new(spanStoreMocks.Writer)
	depReader := new(depStoreMocks.Reader)

	factory := new(factoryMocks.Factory)
	factory.On("CreateSpanReader").Return(reader, nil)
	factory.On("CreateSpanWriter").Return(writer, nil)
	factory.On("CreateDependencyReader").Return(depReader, nil)

	return &storageMocks{
		factory:   factory,
		reader:    reader,
		writer:    writer,
		depReader: depReader,
	}
}

func TestNewServer_TLSConfigError(t *testing.T) {
	tlsCfg := tlscfg.Options{
		Enabled:      true,
		CertPath:     "invalid/path",
		KeyPath:      "invalid/path",
		ClientCAPath: "invalid/path",
	}
	storageMocks := newStorageMocks()
	_, err := NewServer(
		&Options{GRPCHostPort: ":8081", TLSGRPC: tlsCfg},
		storageMocks.factory,
		tenancy.NewManager(&tenancy.Options{}),
		zap.NewNop(),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid TLS config")
}

func TestCreateGRPCHandler(t *testing.T) {
	storageMocks := newStorageMocks()
	h, err := createGRPCHandler(storageMocks.factory, zap.NewNop())
	require.NoError(t, err)

	storageMocks.writer.On("WriteSpan", mock.Anything, mock.Anything).Return(errors.New("writer error"))
	_, err = h.WriteSpan(context.Background(), &storage_v1.WriteSpanRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "writer error")

	storageMocks.depReader.On("GetDependencies", mock.Anything, mock.Anything).Return(nil, errors.New("deps error"))
	_, err = h.GetDependencies(context.Background(), &storage_v1.GetDependenciesRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "deps error")

	err = h.GetArchiveTrace(nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not implemented")

	_, err = h.WriteArchiveSpan(context.Background(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not implemented")

	err = h.WriteSpanStream(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not implemented")
}

var testCases = []struct {
	name              string
	TLS               tlscfg.Options
	clientTLS         tlscfg.Options
	expectError       bool
	expectClientError bool
	expectServerFail  bool
}{
	{
		name: "should pass with insecure connection",
		TLS: tlscfg.Options{
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
		TLS: tlscfg.Options{
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
		TLS: tlscfg.Options{
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
		TLS: tlscfg.Options{
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
		TLS: tlscfg.Options{
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
		TLS: tlscfg.Options{
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
		TLS: tlscfg.Options{
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
}

type grpcClient struct {
	storage_v1.SpanReaderPluginClient

	conn *grpc.ClientConn
}

func newGRPCClient(t *testing.T, addr string, creds credentials.TransportCredentials, tm *tenancy.Manager) *grpcClient {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	dialOpts := []grpc.DialOption{
		grpc.WithUnaryInterceptor(tenancy.NewClientUnaryInterceptor(tm)),
	}
	if creds != nil {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(creds))
	} else {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	conn, err := grpc.DialContext(ctx, addr, dialOpts...)
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
				GRPCHostPort: ":0",
				TLSGRPC:      test.TLS,
			}
			flagsSvc := flags.NewService(ports.QueryAdminHTTP)
			flagsSvc.Logger = zap.NewNop()

			storageMocks := newStorageMocks()
			expectedServices := []string{"test"}
			storageMocks.reader.On("GetServices", mock.AnythingOfType("*context.valueCtx")).Return(expectedServices, nil)

			tm := tenancy.NewManager(&tenancy.Options{Enabled: true})
			server, err := NewServer(
				serverOptions,
				storageMocks.factory,
				tm,
				flagsSvc.Logger,
			)
			assert.Nil(t, err)
			assert.NoError(t, server.Start())

			var wg sync.WaitGroup
			wg.Add(1)
			once := sync.Once{}

			go func() {
				for s := range server.HealthCheckStatus() {
					flagsSvc.HC().Set(s)
					if s == healthcheck.Unavailable {
						once.Do(wg.Done)
					}
				}
			}()

			var clientError error
			var client *grpcClient

			if serverOptions.TLSGRPC.Enabled {
				clientTLSCfg, err0 := test.clientTLS.Config(zap.NewNop())
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
			require.Nil(t, client.conn.Close())
			server.Close()
			wg.Wait()
			assert.Equal(t, healthcheck.Unavailable, flagsSvc.HC().Get())
		})
	}
}

func TestServerHandlesPortZero(t *testing.T) {
	flagsSvc := flags.NewService(ports.QueryAdminHTTP)
	zapCore, logs := observer.New(zap.InfoLevel)
	flagsSvc.Logger = zap.New(zapCore)
	storageMocks := newStorageMocks()

	server, err := NewServer(
		&Options{GRPCHostPort: ":0"},
		storageMocks.factory,
		tenancy.NewManager(&tenancy.Options{}),
		flagsSvc.Logger,
	)
	require.Nil(t, err)
	require.NoError(t, server.Start())
	defer server.Close()

	const line = "Starting GRPC server"
	message := logs.FilterMessage(line)
	require.Equal(t, 1, message.Len(), "Expected '%s' log message, actual logs: %+v", line, logs)

	onlyEntry := message.All()[0]
	hostPort := onlyEntry.ContextMap()["addr"].(string)
	validateGRPCServer(t, hostPort, server.grpcServer)
}

func validateGRPCServer(t *testing.T, hostPort string, server *grpc.Server) {
	grpctest.ReflectionServiceValidator{
		HostPort: hostPort,
		Server:   server,
		ExpectedServices: []string{
			"jaeger.storage.v1.SpanReaderPlugin",
			"jaeger.storage.v1.SpanWriterPlugin",
			"jaeger.storage.v1.DependenciesReaderPlugin",
			"jaeger.storage.v1.PluginCapabilities",
			"jaeger.storage.v1.ArchiveSpanReaderPlugin",
			"jaeger.storage.v1.ArchiveSpanWriterPlugin",
			"jaeger.storage.v1.StreamingSpanWriterPlugin",
			// "grpc.health.v1.Health",
		},
	}.Execute(t)
}
