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
	"net"
	"sync"
	"testing"
	"time"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/jaegertracing/jaeger/cmd/flags"
	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
	"github.com/jaegertracing/jaeger/pkg/healthcheck"
	"github.com/jaegertracing/jaeger/ports"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
	depsmocks "github.com/jaegertracing/jaeger/storage/dependencystore/mocks"
	spanstoremocks "github.com/jaegertracing/jaeger/storage/spanstore/mocks"
)

func TestServerError(t *testing.T) {
	srv := &Server{
		queryOptions: &QueryOptions{
			HostPort: ":-1",
		},
	}
	assert.Error(t, srv.Start())
}

func TestCreateTLSServerError(t *testing.T) {
	tlsCfg := tlscfg.Options{
		Enabled:      true,
		CertPath:     "invalid/path",
		KeyPath:      "invalid/path",
		ClientCAPath: "invalid/path",
	}

	_, err := NewServer(zap.NewNop(), &querysvc.QueryService{},
		&QueryOptions{TLS: tlsCfg}, opentracing.NoopTracer{})
	assert.NotNil(t, err)
}

func TestServerBadHostPort(t *testing.T) {
	_, err := NewServer(zap.NewNop(), &querysvc.QueryService{},
		&QueryOptions{HTTPHostPort: "8080", GRPCHostPort: "127.0.0.1:8081", BearerTokenPropagation: true},
		opentracing.NoopTracer{})

	assert.NotNil(t, err)
	_, err = NewServer(zap.NewNop(), &querysvc.QueryService{},
		&QueryOptions{HTTPHostPort: "127.0.0.1:8081", GRPCHostPort: "9123", BearerTokenPropagation: true},
		opentracing.NoopTracer{})

	assert.NotNil(t, err)
}

func TestServerInUseHostPort(t *testing.T) {
	const availableHostPort = "127.0.0.1:0"
	conn, err := net.Listen("tcp", availableHostPort)
	require.NoError(t, err)
	defer func() { require.NoError(t, conn.Close()) }()

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
				zap.NewNop(),
				&querysvc.QueryService{},
				&QueryOptions{
					HTTPHostPort:           tc.httpHostPort,
					GRPCHostPort:           tc.grpcHostPort,
					BearerTokenPropagation: true,
				},
				opentracing.NoopTracer{},
			)
			assert.NoError(t, err)

			err = server.Start()
			assert.Error(t, err)

			if server.grpcConn != nil {
				server.grpcConn.Close()
			}
			if server.httpConn != nil {
				server.httpConn.Close()
			}
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
		&QueryOptions{HostPort: hostPort, GRPCHostPort: hostPort, HTTPHostPort: hostPort, BearerTokenPropagation: true},
		opentracing.NoopTracer{})
	assert.Nil(t, err)
	assert.NoError(t, server.Start())

	var wg sync.WaitGroup
	wg.Add(1)
	once := sync.Once{}

	go func() {
		for s := range server.HealthCheckStatus() {
			flagsSvc.HC().Set(s)
			if s == healthcheck.Unavailable {
				once.Do(func() {
					wg.Done()
				})
			}

		}
		wg.Done()

	}()

	client := newGRPCClient(t, hostPort)
	defer client.conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	res, err := client.GetServices(ctx, &api_v2.GetServicesRequest{})
	assert.NoError(t, err)
	assert.Equal(t, expectedServices, res.Services)

	server.Close()
	wg.Wait()
	assert.Equal(t, healthcheck.Unavailable, flagsSvc.HC().Get())
}

func TestServerWithDedicatedPorts(t *testing.T) {
	flagsSvc := flags.NewService(ports.QueryAdminHTTP)
	flagsSvc.Logger = zap.NewNop()

	spanReader := &spanstoremocks.Reader{}
	dependencyReader := &depsmocks.Reader{}
	expectedServices := []string{"test"}
	spanReader.On("GetServices", mock.AnythingOfType("*context.valueCtx")).Return(expectedServices, nil)

	querySvc := querysvc.NewQueryService(spanReader, dependencyReader, querysvc.QueryServiceOptions{})

	server, err := NewServer(flagsSvc.Logger, querySvc,
		&QueryOptions{HTTPHostPort: "127.0.0.1:8080", GRPCHostPort: "127.0.0.1:8081", BearerTokenPropagation: true},
		opentracing.NoopTracer{})
	assert.Nil(t, err)
	assert.NoError(t, server.Start())

	var wg sync.WaitGroup
	wg.Add(1)
	once := sync.Once{}

	go func() {
		for s := range server.HealthCheckStatus() {
			flagsSvc.HC().Set(s)
			if s == healthcheck.Unavailable {
				once.Do(func() {
					wg.Done()
				})
			}
		}
	}()

	client := newGRPCClient(t, "127.0.0.1:8081")
	defer client.conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	res, err := client.GetServices(ctx, &api_v2.GetServicesRequest{})
	assert.NoError(t, err)
	assert.Equal(t, expectedServices, res.Services)

	server.Close()
	wg.Wait()
	assert.Equal(t, healthcheck.Unavailable, flagsSvc.HC().Get())
}

func TestServerGracefulExit(t *testing.T) {
	flagsSvc := flags.NewService(ports.QueryAdminHTTP)

	zapCore, logs := observer.New(zap.ErrorLevel)
	assert.Equal(t, 0, logs.Len(), "Expected initial ObservedLogs to have zero length.")

	flagsSvc.Logger = zap.New(zapCore)
	hostPort := ports.PortToHostPort(ports.QueryAdminHTTP)

	querySvc := &querysvc.QueryService{}
	tracer := opentracing.NoopTracer{}
	server, err := NewServer(flagsSvc.Logger, querySvc, &QueryOptions{HostPort: hostPort, GRPCHostPort: hostPort, HTTPHostPort: hostPort}, tracer)
	assert.Nil(t, err)
	assert.NoError(t, server.Start())
	go func() {
		for s := range server.HealthCheckStatus() {
			flagsSvc.HC().Set(s)
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
	server, err := NewServer(flagsSvc.Logger, querySvc, &QueryOptions{HostPort: ":0", GRPCHostPort: ":0", HTTPHostPort: ":0"}, tracer)
	assert.Nil(t, err)
	assert.NoError(t, server.Start())
	server.Close()

	message := logs.FilterMessage("Query server started")
	assert.Equal(t, 1, message.Len(), "Expected query started log message.")

	onlyEntry := message.All()[0]
	port := onlyEntry.ContextMap()["port"]
	assert.Greater(t, port, int64(0))
}
