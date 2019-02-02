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

package grpcserver

import (
	"fmt"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"io/ioutil"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/pkg/errors"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/grpclog"

	"github.com/jaegertracing/jaeger/cmd/collector/app"
	"github.com/jaegertracing/jaeger/cmd/collector/app/sampling"
	"github.com/jaegertracing/jaeger/cmd/collector/app/sampling/strategystore"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
)

// StartGRPCCollector configures and starts gRPC endpoints exposed by collector.
func StartGRPCCollector(
	port int,
	server *grpc.Server,
	tlsCertPath, tlsKeyPath string,
	handler *app.GRPCHandler,
	samplingStrategy strategystore.StrategyStore,
	logger *zap.Logger,
	serveErr func(error),
) (net.Addr, error) {
	grpcPortStr := ":" + strconv.Itoa(port)
	lis, err := net.Listen("tcp", grpcPortStr)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to listen on gRPC port")
	}
	grpclog.SetLoggerV2(grpclog.NewLoggerV2(ioutil.Discard, os.Stderr, os.Stderr))

	//use TLS only if certificates are provided
	if tlsCertPath != "" && tlsKeyPath != "" {
		creds, err := credentials.NewServerTLSFromFile(tlsCertPath, tlsKeyPath)
		if err != nil {
			logger.Fatal(fmt.Sprintf("Failed to setup tls: %v", err))
		}
		opts := []grpc.ServerOption{
			grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
				MinTime:             1 * time.Minute,
				PermitWithoutStream: true,
			}),
			grpc.Creds(creds),
		}
		server = grpc.NewServer(opts...)
		logger.Info("initialising grpc with certificates")
	}

	api_v2.RegisterCollectorServiceServer(server, handler)
	api_v2.RegisterSamplingManagerServer(server, sampling.NewGRPCHandler(samplingStrategy))

	startServer(server, lis, logger, serveErr)
	return lis.Addr(), nil
}

func startServer(server *grpc.Server, lis net.Listener, logger *zap.Logger, serveErr func(error)) {
	var port string
	if tcpAddr, ok := lis.Addr().(*net.TCPAddr); ok {
		port = strconv.Itoa(tcpAddr.Port)
	} else {
		port = lis.Addr().Network()
	}
	logger.Info("Starting jaeger-collector gRPC server", zap.String("grpc-port", port))
	go func() {
		if err := server.Serve(lis); err != nil {
			logger.Error("Could not launch gRPC service", zap.Error(err))
			serveErr(err)
		}
	}()
}
