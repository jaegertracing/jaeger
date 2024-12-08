// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/confignet"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"github.com/jaegertracing/jaeger/cmd/collector/app/handler"
	"github.com/jaegertracing/jaeger/internal/grpctest"
	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
)

// test wrong port number
func TestFailToListen(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	server, err := StartGRPCServer(&GRPCServerParams{
		ServerConfig: configgrpc.ServerConfig{
			NetAddr: confignet.AddrConfig{
				Endpoint:  ":-1",
				Transport: confignet.TransportTypeTCP,
			},
		},
		Handler:          handler.NewGRPCHandler(logger, &mockSpanProcessor{}, &tenancy.Manager{}),
		SamplingProvider: &mockSamplingProvider{},
		Logger:           logger,
	})
	assert.Nil(t, server)
	require.EqualError(t, err, "failed to listen on gRPC port: listen tcp: address -1: invalid port")
}

func TestFailServe(t *testing.T) {
	lis := bufconn.Listen(0)
	lis.Close()
	core, logs := observer.New(zap.NewAtomicLevelAt(zapcore.ErrorLevel))
	var wg sync.WaitGroup
	wg.Add(1)

	logger := zap.New(core)
	server := grpc.NewServer()
	defer server.Stop()
	serveGRPC(server, lis, &GRPCServerParams{
		Handler:          handler.NewGRPCHandler(logger, &mockSpanProcessor{}, &tenancy.Manager{}),
		SamplingProvider: &mockSamplingProvider{},
		Logger:           logger,
		OnError: func(_ error) {
			assert.Len(t, logs.All(), 1)
			assert.Equal(t, "Could not launch gRPC service", logs.All()[0].Message)
			wg.Done()
		},
	})
	wg.Wait()
}

func TestSpanCollector(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	params := &GRPCServerParams{
		Handler:          handler.NewGRPCHandler(logger, &mockSpanProcessor{}, &tenancy.Manager{}),
		SamplingProvider: &mockSamplingProvider{},
		Logger:           logger,
		ServerConfig: configgrpc.ServerConfig{
			MaxRecvMsgSizeMiB: 2,
			NetAddr: confignet.AddrConfig{
				Transport: confignet.TransportTypeTCP,
			},
		},
	}

	server, err := StartGRPCServer(params)
	require.NoError(t, err)
	defer server.Stop()

	conn, err := grpc.NewClient(
		params.HostPortActual,
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()

	c := api_v2.NewCollectorServiceClient(conn)
	response, err := c.PostSpans(context.Background(), &api_v2.PostSpansRequest{})
	require.NoError(t, err)
	require.NotNil(t, response)
}

func TestCollectorStartWithTLS(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	opts := tlscfg.Options{
		Enabled:      true,
		CertPath:     testCertKeyLocation + "/example-server-cert.pem",
		KeyPath:      testCertKeyLocation + "/example-server-key.pem",
		ClientCAPath: testCertKeyLocation + "/example-CA-cert.pem",
	}
	params := &GRPCServerParams{
		Handler:          handler.NewGRPCHandler(logger, &mockSpanProcessor{}, &tenancy.Manager{}),
		SamplingProvider: &mockSamplingProvider{},
		Logger:           logger,
		ServerConfig: configgrpc.ServerConfig{
			NetAddr: confignet.AddrConfig{
				Transport: confignet.TransportTypeTCP,
			},
			TLSSetting: opts.ToOtelServerConfig(),
		},
	}
	server, err := StartGRPCServer(params)
	require.NoError(t, err)
	defer server.Stop()
}

func TestCollectorReflection(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	params := &GRPCServerParams{
		Handler:          handler.NewGRPCHandler(logger, &mockSpanProcessor{}, &tenancy.Manager{}),
		SamplingProvider: &mockSamplingProvider{},
		Logger:           logger,
		ServerConfig: configgrpc.ServerConfig{
			NetAddr: confignet.AddrConfig{
				Transport: confignet.TransportTypeTCP,
			},
		},
	}

	server, err := StartGRPCServer(params)
	require.NoError(t, err)
	defer server.Stop()

	grpctest.ReflectionServiceValidator{
		HostPort: params.HostPortActual,
		ExpectedServices: []string{
			"jaeger.api_v2.CollectorService",
			"jaeger.api_v2.SamplingManager",
			"grpc.health.v1.Health",
		},
	}.Execute(t)
}
