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
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strconv"

	"github.com/pkg/errors"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/grpclog"

	"github.com/jaegertracing/jaeger/cmd/collector/app"
	"github.com/jaegertracing/jaeger/cmd/collector/app/sampling"
	"github.com/jaegertracing/jaeger/cmd/collector/app/sampling/strategystore"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
)

// TLSConfig creates a *tls.Config from the user specified file paths.
func TLSConfig(cert, key, clientCA string) (*tls.Config, error) {
	if cert == "" || key == "" {
		return nil, fmt.Errorf("you requested TLS but configuration does not include a path to cert and/or key")
	}

	tlsCfg := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	tlsCert, err := tls.LoadX509KeyPair(filepath.Clean(cert), filepath.Clean(key))
	if err != nil {
		return nil, fmt.Errorf("could not load server TLS cert and key, %v", err)
	}

	tlsCfg.Certificates = []tls.Certificate{tlsCert}

	if clientCA != "" {
		caPEM, err := ioutil.ReadFile(filepath.Clean(clientCA))
		if err != nil {
			return nil, fmt.Errorf("load TLS client CA, %v", err)
		}

		certPool := x509.NewCertPool()
		if !certPool.AppendCertsFromPEM(caPEM) {
			return nil, fmt.Errorf("building TLS client CA, %v", err)
		}
		tlsCfg.ClientCAs = certPool
		tlsCfg.ClientAuth = tls.RequireAndVerifyClientCert
	}

	return tlsCfg, nil
}

// StartGRPCCollector configures and starts gRPC endpoints exposed by collector.
func StartGRPCCollector(
	port int,
	server *grpc.Server,
	handler *app.GRPCHandler,
	samplingStrategy strategystore.StrategyStore,
	logger *zap.Logger,
	serveErr func(error),
) (net.Addr, error) {
	grpcPortStr := ":" + strconv.Itoa(port)
	lis, err := net.Listen("tcp", grpcPortStr)
	if err != nil {
		return nil, errors.Wrap(err, "failed to listen on gRPC port")
	}

	grpclog.SetLoggerV2(grpclog.NewLoggerV2(ioutil.Discard, os.Stderr, os.Stderr))

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
