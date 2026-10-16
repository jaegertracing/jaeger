// Copyright (c) 2022 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configauth"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/confignet"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/config/configtls"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/encoding/proto"
	"google.golang.org/grpc/mem"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/jaegertracing/jaeger/internal/grpctest"
	"github.com/jaegertracing/jaeger/internal/proto-gen/storage/v2"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	tracestoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore/mocks"
	"github.com/jaegertracing/jaeger/internal/telemetry"
	"github.com/jaegertracing/jaeger/internal/tenancy"
)

var testCertKeyLocation = "../../../internal/config/tlscfg/testdata"

func TestNewServer_CreateStorageErrors(t *testing.T) {
	createServer := func(factory *fakeFactory) (*Server, error) {
		return NewServer(
			context.Background(),
			configgrpc.ServerConfig{
				NetAddr: confignet.AddrConfig{
					Endpoint: ":0",
				},
			},
			factory,
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
	require.NoError(t, s.Start(context.Background()))
	validateGRPCServer(t, s.GRPCAddr())
	require.NoError(t, s.grpcConn.Close())
}

func TestServerStart_BadPortErrors(t *testing.T) {
	srv := &Server{
		grpcCfg: configgrpc.ServerConfig{
			NetAddr: confignet.AddrConfig{
				Endpoint: ":-1",
			},
		},
	}
	require.Error(t, srv.Start(context.Background()))
}

type fakeFactory struct {
	reader    tracestore.Reader
	writer    tracestore.Writer
	depReader depstore.Reader

	readerErr    error
	writerErr    error
	depReaderErr error
}

func (f *fakeFactory) CreateTraceReader() (tracestore.Reader, error) {
	if f.readerErr != nil {
		return nil, f.readerErr
	}
	return f.reader, nil
}

func (f *fakeFactory) CreateTraceWriter() (tracestore.Writer, error) {
	if f.writerErr != nil {
		return nil, f.writerErr
	}
	return f.writer, nil
}

func (f *fakeFactory) CreateDependencyReader() (depstore.Reader, error) {
	if f.depReaderErr != nil {
		return nil, f.depReaderErr
	}
	return f.depReader, nil
}

func (*fakeFactory) InitArchiveStorage(*zap.Logger) (spanstore.Reader, spanstore.Writer) {
	return nil, nil
}

func TestNewServer_TLSConfigError(t *testing.T) {
	tlsCfg := configtls.ServerConfig{
		ClientCAFile: "invalid/path",
		Config: configtls.Config{
			CertFile: "invalid/path",
			KeyFile:  "invalid/path",
		},
	}
	telset := telemetry.NoopSettings()
	telset.Logger = zap.NewNop()

	_, err := NewServer(
		context.Background(),
		configgrpc.ServerConfig{
			NetAddr: confignet.AddrConfig{
				Endpoint: ":8081",
			},
			TLS: configoptional.Some(tlsCfg),
		},
		&fakeFactory{},
		&fakeFactory{},
		tenancy.NewManager(&tenancy.Options{}),
		telset,
	)
	assert.ErrorContains(t, err, "failed to load TLS config")
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
	storage.TraceReaderClient

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
		TraceReaderClient: storage.NewTraceReaderClient(conn),
		conn:              conn,
	}
}

// decodeCounter is the stock codec under a content subtype of its own, counting what it decodes.
// The server finds a codec in the global registry by the subtype the client sends, so a client
// forcing this subtype gets its requests decoded by serverDecodes.
type decodeCounter struct {
	encoding.CodecV2

	decoded atomic.Int64
}

func (*decodeCounter) Name() string {
	return "decode-counter"
}

func (c *decodeCounter) Unmarshal(data mem.BufferSlice, v any) error {
	c.decoded.Add(1)
	return c.CodecV2.Unmarshal(data, v)
}

var serverDecodes = &decodeCounter{CodecV2: encoding.GetCodecV2(proto.Name)}

func init() {
	encoding.RegisterCodecV2(serverDecodes)
}

func TestServerChecksTenantBeforeDecodingRequest(t *testing.T) {
	type traceIDsChunk = tracestore.PageChunk[[]tracestore.FoundTraceID]
	traceID := pcommon.TraceID{1}
	reader := new(tracestoremocks.Reader)
	reader.On("FindTraceIDs", mock.Anything, mock.Anything).Return(
		func(ctx context.Context, _ tracestore.TraceQueryParams) iter.Seq2[traceIDsChunk, error] {
			return func(yield func(traceIDsChunk, error) bool) {
				if tenant := tenancy.GetTenant(ctx); tenant != "acme" {
					yield(traceIDsChunk{}, fmt.Errorf("reader got tenant %q", tenant))
					return
				}
				yield(traceIDsChunk{Results: []tracestore.FoundTraceID{{TraceID: traceID}}}, nil)
			}
		})
	f := &fakeFactory{reader: reader}
	tm := tenancy.NewManager(&tenancy.Options{Enabled: true, Tenants: []string{"acme"}})
	server, err := NewServer(
		context.Background(),
		configgrpc.ServerConfig{
			NetAddr: confignet.AddrConfig{Endpoint: ":0"},
		},
		f,
		f,
		tm,
		telemetry.NoopSettings(),
	)
	require.NoError(t, err)
	require.NoError(t, server.Start(context.Background()))
	defer server.Close()
	client := newGRPCClient(t, server.GRPCAddr(), nil, tm)
	defer client.conn.Close()

	tests := []struct {
		name     string
		tenants  []string
		wantCode codes.Code
	}{
		{name: "missing tenant", wantCode: codes.Unauthenticated},
		{name: "unknown tenant", tenants: []string{"megacorp"}, wantCode: codes.PermissionDenied},
		{name: "extra tenant", tenants: []string{"acme", "megacorp"}, wantCode: codes.PermissionDenied},
		{name: "valid tenant", tenants: []string{"acme"}, wantCode: codes.OK},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if test.tenants != nil {
				ctx = metadata.NewOutgoingContext(ctx, metadata.MD{tm.Header: test.tenants})
			}
			decodedBefore := serverDecodes.decoded.Load()
			res, err := client.FindTraceIDs(
				ctx,
				&storage.FindTraceIDsRequest{Query: &storage.TraceQueryParameters{ServiceName: "service"}},
				grpc.ForceCodecV2(&decodeCounter{CodecV2: encoding.GetCodecV2(proto.Name)}),
			)
			decoded := serverDecodes.decoded.Load() - decodedBefore
			if test.wantCode != codes.OK {
				require.Equalf(t, test.wantCode, status.Code(err), "error: %v", err)
				assert.Zero(t, decoded, "the server decoded a request whose tenant it rejects")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, int64(1), decoded)
			require.Len(t, res.TraceIds, 1)
			assert.Equal(t, traceID[:], res.TraceIds[0].TraceId)
		})
	}
}

var errNoCredentials = errors.New("no credentials")

// rejectingAuthenticator is a server authenticator extension that turns every caller away.
type rejectingAuthenticator struct {
	component.StartFunc
	component.ShutdownFunc
}

func (rejectingAuthenticator) Authenticate(ctx context.Context, _ map[string][]string) (context.Context, error) {
	return ctx, errNoCredentials
}

// extensionsHost is a component.Host that offers the extensions it holds.
type extensionsHost map[component.ID]component.Component

func (h extensionsHost) GetExtensions() map[component.ID]component.Component {
	return h
}

func TestServerAuthenticatesBeforeCheckingTenant(t *testing.T) {
	authID := component.MustNewID("rejecting_auth")
	telset := telemetry.NoopSettings()
	telset.Host = extensionsHost{authID: rejectingAuthenticator{}}
	tm := tenancy.NewManager(&tenancy.Options{Enabled: true, Tenants: []string{"acme"}})
	f := &fakeFactory{reader: new(tracestoremocks.Reader)}
	server, err := NewServer(
		context.Background(),
		configgrpc.ServerConfig{
			NetAddr: confignet.AddrConfig{Endpoint: ":0"},
			Auth:    configoptional.Some(configauth.Config{AuthenticatorID: authID}),
		},
		f,
		f,
		tm,
		telset,
	)
	require.NoError(t, err)
	require.NoError(t, server.Start(context.Background()))
	defer server.Close()
	client := newGRPCClient(t, server.GRPCAddr(), nil, tm)
	defer client.conn.Close()

	tests := []struct {
		name    string
		tenants []string
	}{
		{name: "missing tenant"},
		{name: "unknown tenant", tenants: []string{"megacorp"}},
		{name: "valid tenant", tenants: []string{"acme"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if test.tenants != nil {
				ctx = metadata.NewOutgoingContext(ctx, metadata.MD{tm.Header: test.tenants})
			}
			_, err := client.FindTraceIDs(
				ctx,
				&storage.FindTraceIDsRequest{Query: &storage.TraceQueryParameters{ServiceName: "service"}},
			)
			// Every call must be answered by the authenticator, so the status does not depend on the tenant.
			st := status.Convert(err)
			assert.Equal(t, codes.Unauthenticated, st.Code())
			assert.Equal(t, errNoCredentials.Error(), st.Message())
		})
	}
}

func TestServerGRPCTLS(t *testing.T) {
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			tls := configoptional.None[configtls.ServerConfig]()
			if test.TLS != nil {
				tls = configoptional.Some(*test.TLS)
			}
			serverOptions := configgrpc.ServerConfig{
				NetAddr: confignet.AddrConfig{
					Endpoint: ":0",
				},
				TLS: tls,
			}

			reader := new(tracestoremocks.Reader)
			f := &fakeFactory{
				reader: reader,
			}
			expectedServices := []string{"test"}
			reader.On("GetServices", mock.AnythingOfType("*context.valueCtx")).Return(expectedServices, nil)

			tm := tenancy.NewManager(&tenancy.Options{Enabled: true})
			telset := telemetry.NoopSettings()
			telset.Logger = zap.NewNop()
			server, err := NewServer(
				context.Background(),
				serverOptions,
				f,
				f,
				tm,
				telset,
			)
			require.NoError(t, err)
			require.NoError(t, server.Start(context.Background()))

			var clientError error
			var client *grpcClient

			if serverOptions.TLS.HasValue() {
				clientTLSCfg, err0 := test.clientTLS.LoadTLSConfig(context.Background())
				require.NoError(t, err0)
				creds := credentials.NewTLS(clientTLSCfg)
				client = newGRPCClient(t, server.GRPCAddr(), creds, tm)
			} else {
				client = newGRPCClient(t, server.GRPCAddr(), nil, tm)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			ctx = tenancy.WithTenant(ctx, "foo")
			res, clientError := client.GetServices(ctx, &storage.GetServicesRequest{})

			if test.expectClientError {
				require.Error(t, clientError)
			} else {
				require.NoError(t, clientError)
				assert.Equal(t, expectedServices, res.Services)
			}
			require.NoError(t, client.conn.Close())
			server.Close()
		})
	}
}

func TestServerHandlesPortZero(t *testing.T) {
	zapCore, logs := observer.New(zap.InfoLevel)
	logger := zap.New(zapCore)
	telset := telemetry.NoopSettings()
	telset.Logger = logger
	server, err := NewServer(
		context.Background(),
		configgrpc.ServerConfig{
			NetAddr: confignet.AddrConfig{Endpoint: ":0"},
		},
		&fakeFactory{},
		&fakeFactory{},
		tenancy.NewManager(&tenancy.Options{}),
		telset,
	)
	require.NoError(t, err)

	require.NoError(t, server.Start(context.Background()))

	const line = "Starting GRPC server"
	message := logs.FilterMessage(line)
	require.Equal(t, 1, message.Len(), "Expected '%s' log message, actual logs: %+v", line, logs)

	onlyEntry := message.All()[0]
	hostPort := onlyEntry.ContextMap()["addr"].(string)
	validateGRPCServer(t, hostPort)

	server.Close()
}

func validateGRPCServer(t *testing.T, hostPort string) {
	grpctest.ReflectionServiceValidator{
		HostPort: hostPort,
		ExpectedServices: []string{
			// writer
			"opentelemetry.proto.collector.trace.v1.TraceService",
			// reader
			"jaeger.storage.v2.TraceReader",
			"jaeger.storage.v2.DependencyReader",
			// health
			"grpc.health.v1.Health",
		},
	}.Execute(t)
}
