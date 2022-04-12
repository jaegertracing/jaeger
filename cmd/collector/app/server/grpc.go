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

package server

import (
	"fmt"
	"net"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"

	"github.com/jaegertracing/jaeger/cmd/collector/app/handler"
	"github.com/jaegertracing/jaeger/cmd/collector/app/sampling"
	"github.com/jaegertracing/jaeger/cmd/collector/app/sampling/strategystore"
	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
)

// GRPCServerParams to construct a new Jaeger Collector gRPC Server
type GRPCServerParams struct {
	TLSConfig               tlscfg.Options
	HostPort                string
	Handler                 *handler.GRPCHandler
	SamplingStore           strategystore.StrategyStore
	Logger                  *zap.Logger
	OnError                 func(error)
	MaxReceiveMessageLength int
	MaxConnectionAge        time.Duration
	MaxConnectionAgeGrace   time.Duration

	// Set by the server to indicate the actual host:port of the server.
	HostPortActual string
}

// StartGRPCServer based on the given parameters
func StartGRPCServer(params *GRPCServerParams) (*grpc.Server, error) {
	var server *grpc.Server
	var grpcOpts []grpc.ServerOption

	if params.MaxReceiveMessageLength > 0 {
		grpcOpts = append(grpcOpts, grpc.MaxRecvMsgSize(params.MaxReceiveMessageLength))
	}
	grpcOpts = append(grpcOpts, grpc.KeepaliveParams(keepalive.ServerParameters{
		MaxConnectionAge:      params.MaxConnectionAge,
		MaxConnectionAgeGrace: params.MaxConnectionAgeGrace,
	}))

	if params.TLSConfig.Enabled {
		// user requested a server with TLS, setup creds
		tlsCfg, err := params.TLSConfig.Config(params.Logger)
		if err != nil {
			return nil, err
		}

		creds := credentials.NewTLS(tlsCfg)
		grpcOpts = append(grpcOpts, grpc.Creds(creds))
	}

	server = grpc.NewServer(grpcOpts...)
	reflection.Register(server)

	listener, err := net.Listen("tcp", params.HostPort)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on gRPC port: %w", err)
	}
	params.HostPortActual = listener.Addr().String()

	if err := serveGRPC(server, listener, params); err != nil {
		return nil, err
	}

	return server, nil
}

func serveGRPC(server *grpc.Server, listener net.Listener, params *GRPCServerParams) error {
	api_v2.RegisterCollectorServiceServer(server, params.Handler)
	api_v2.RegisterSamplingManagerServer(server, sampling.NewGRPCHandler(params.SamplingStore))

	params.Logger.Info("Starting jaeger-collector gRPC server", zap.String("grpc.host-port", params.HostPortActual))
	go func() {
		if err := server.Serve(listener); err != nil {
			params.Logger.Error("Could not launch gRPC service", zap.Error(err))
			if params.OnError != nil {
				params.OnError(err)
			}
		}
	}()

	return nil
}
