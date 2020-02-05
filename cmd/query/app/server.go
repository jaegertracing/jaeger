// Copyright (c) 2019 The Jaeger Authors.
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
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/textproto"
	"strings"

	"github.com/gorilla/handlers"
	"github.com/opentracing/opentracing-go"
	"github.com/soheilhy/cmux"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	"github.com/jaegertracing/jaeger/cmd/flags"
	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	"github.com/jaegertracing/jaeger/pkg/healthcheck"
	"github.com/jaegertracing/jaeger/pkg/netutils"
	"github.com/jaegertracing/jaeger/pkg/recoveryhandler"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
)

// Server runs HTTP, Mux and a grpc server
type Server struct {
	svc          *flags.Service
	querySvc     *querysvc.QueryService
	queryOptions *QueryOptions

	tracer opentracing.Tracer // TODO make part of flags.Service

	conn       net.Listener
	grpcServer *grpc.Server
	httpServer *http.Server
}

// NewServer creates and initializes Server
func NewServer(svc *flags.Service, querySvc *querysvc.QueryService, options *QueryOptions, tracer opentracing.Tracer) *Server {
	return &Server{
		svc:          svc,
		querySvc:     querySvc,
		queryOptions: options,
		tracer:       tracer,
		grpcServer:   createGRPCServer(querySvc, svc.Logger, tracer),
		httpServer:   createHTTPServer(querySvc, options, tracer, svc.Logger),
	}
}

func createGRPCServer(querySvc *querysvc.QueryService, logger *zap.Logger, tracer opentracing.Tracer) *grpc.Server {
	srv := grpc.NewServer()
	handler := NewGRPCHandler(querySvc, logger, tracer)
	api_v2.RegisterQueryServiceServer(srv, handler)
	return srv
}

func createHTTPServer(querySvc *querysvc.QueryService, queryOpts *QueryOptions, tracer opentracing.Tracer, logger *zap.Logger) *http.Server {
	headers := stringSliceAsHeader(queryOpts.AdditionalHeaders, logger)
	apiHandlerOptions := []HandlerOption{
		HandlerOptions.Logger(logger),
		HandlerOptions.Tracer(tracer),
		HandlerOptions.AdditionalHeaders(headers),
	}
	apiHandler := NewAPIHandler(
		querySvc,
		apiHandlerOptions...)
	r := NewRouter()
	if queryOpts.BasePath != "/" {
		r = r.PathPrefix(queryOpts.BasePath).Subrouter()
	}

	apiHandler.RegisterRoutes(r)
	RegisterStaticHandler(r, logger, queryOpts)
	var handler http.Handler = r
	if queryOpts.BearerTokenPropagation {
		handler = bearerTokenPropagationHandler(logger, r)
	}
	handler = handlers.CompressHandler(handler)
	recoveryHandler := recoveryhandler.NewRecoveryHandler(logger, true)
	return &http.Server{
		Handler: recoveryHandler(handler),
	}
}

// Start http, GRPC and cmux servers concurrently
func (s *Server) Start() error {
	conn, err := net.Listen("tcp", fmt.Sprintf(":%d", s.queryOptions.Port))
	if err != nil {
		return err
	}
	s.conn = conn

	tcpPort := s.queryOptions.Port
	if port, err := netutils.GetPort(s.conn.Addr()); err == nil {
		tcpPort = port
	}

	s.svc.Logger.Info(
		"Query server started",
		zap.Int("port", tcpPort))

	// cmux server acts as a reverse-proxy between HTTP and GRPC backends.
	cmuxServer := cmux.New(s.conn)

	grpcListener := cmuxServer.MatchWithWriters(
		cmux.HTTP2MatchHeaderFieldSendSettings("content-type", "application/grpc"),
		cmux.HTTP2MatchHeaderFieldSendSettings("content-type", "application/grpc+proto"),
	)
	httpListener := cmuxServer.Match(cmux.Any())

	go func() {
		s.svc.Logger.Info("Starting HTTP server", zap.Int("port", tcpPort))

		switch err := s.httpServer.Serve(httpListener); err {
		case nil, http.ErrServerClosed, cmux.ErrListenerClosed:
			// normal exit, nothing to do
		default:
			s.svc.Logger.Error("Could not start HTTP server", zap.Error(err))
		}
		s.svc.SetHealthCheckStatus(healthcheck.Unavailable)
	}()

	// Start GRPC server concurrently
	go func() {
		s.svc.Logger.Info("Starting GRPC server", zap.Int("port", tcpPort))

		if err := s.grpcServer.Serve(grpcListener); err != nil {
			s.svc.Logger.Error("Could not start GRPC server", zap.Error(err))
		}
		s.svc.SetHealthCheckStatus(healthcheck.Unavailable)
	}()

	// Start cmux server concurrently.
	go func() {
		s.svc.Logger.Info("Starting CMUX server", zap.Int("port", tcpPort))

		err := cmuxServer.Serve()
		// TODO: Remove string comparison when https://github.com/soheilhy/cmux/pull/69 is merged
		if err != nil && !strings.Contains(err.Error(), "use of closed network connection") {
			s.svc.Logger.Error("Could not start multiplexed server", zap.Error(err))
		}
		s.svc.SetHealthCheckStatus(healthcheck.Unavailable)
	}()

	return nil
}

// Close stops http, GRPC servers and closes the port listener.
func (s *Server) Close() {
	s.grpcServer.Stop()
	s.httpServer.Close()
	s.conn.Close()
}

// stringSliceAsHeader parses a slice of strings and returns a http.Header.
//  Each string in the slice is expected to be in the format "key: value"
func stringSliceAsHeader(slice []string, logger *zap.Logger) http.Header {
	if len(slice) == 0 {
		return nil
	}

	allHeaders := strings.Join(slice, "\r\n")

	reader := bufio.NewReader(strings.NewReader(allHeaders))
	tp := textproto.NewReader(reader)

	header, err := tp.ReadMIMEHeader()
	if err != nil && err != io.EOF {
		logger.Error("Failed to parse headers", zap.Strings("headers", slice))
		return nil
	}

	return http.Header(header)
}
