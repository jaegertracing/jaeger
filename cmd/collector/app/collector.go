// Copyright (c) 2020 The Jaeger Authors.
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
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/cors"
	"github.com/uber/jaeger-lib/metrics"
	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/thrift"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/jaegertracing/jaeger/cmd/collector/app/grpcserver"
	"github.com/jaegertracing/jaeger/cmd/collector/app/handler"
	"github.com/jaegertracing/jaeger/cmd/collector/app/sampling"
	"github.com/jaegertracing/jaeger/cmd/collector/app/sampling/strategystore"
	"github.com/jaegertracing/jaeger/cmd/collector/app/zipkin"
	clientcfgHandler "github.com/jaegertracing/jaeger/pkg/clientcfg/clientcfghttp"
	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
	"github.com/jaegertracing/jaeger/pkg/healthcheck"
	"github.com/jaegertracing/jaeger/pkg/recoveryhandler"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	jc "github.com/jaegertracing/jaeger/thrift-gen/jaeger"
	sc "github.com/jaegertracing/jaeger/thrift-gen/sampling"
	zc "github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
)

// Collector returns the collector as a manageable unit of work
type Collector struct {
	// required to start a new collector
	serviceName    string
	logger         *zap.Logger
	metricsFactory metrics.Factory
	spanWriter     spanstore.Writer
	strategyStore  strategystore.StrategyStore
	hCheck         *healthcheck.HealthCheck

	// state, read only
	hServer    *http.Server
	zkServer   *http.Server
	grpcServer *grpc.Server
	tchServer  *tchannel.Channel
}

// CollectorParams to construct a new Jaeger Collector.
type CollectorParams struct {
	ServiceName    string
	Logger         *zap.Logger
	MetricsFactory metrics.Factory
	SpanWriter     spanstore.Writer
	StrategyStore  strategystore.StrategyStore
	HealthCheck    *healthcheck.HealthCheck
}

// New constructs a new collector component, ready to be started
func New(params *CollectorParams) *Collector {
	return &Collector{
		serviceName:    params.ServiceName,
		logger:         params.Logger,
		metricsFactory: params.MetricsFactory,
		spanWriter:     params.SpanWriter,
		strategyStore:  params.StrategyStore,
		hCheck:         params.HealthCheck,
	}
}

// Start the component and underlying dependencies
func (c *Collector) Start(builderOpts *CollectorOptions) error {
	handlerBuilder := &SpanHandlerBuilder{
		SpanWriter:     c.spanWriter,
		CollectorOpts:  *builderOpts,
		Logger:         c.logger,
		MetricsFactory: c.metricsFactory,
	}
	zipkinSpansHandler, jaegerBatchesHandler, grpcHandler := handlerBuilder.BuildHandlers()
	recoveryHandler := recoveryhandler.NewRecoveryHandler(c.logger, true)

	if tchServer, err := c.StartThriftServer(&ThriftServerParams{
		ServiceName:          c.serviceName,
		Port:                 builderOpts.CollectorPort,
		JaegerBatchesHandler: jaegerBatchesHandler,
		ZipkinSpansHandler:   zipkinSpansHandler,
		StrategyStore:        c.strategyStore,
		Logger:               c.logger,
	}); err != nil {
		c.logger.Fatal("Could not start Thrift collector", zap.Error(err))
	} else {
		c.tchServer = tchServer
	}

	if grpcServer, err := c.StartGRPCServer(&GRPCServerParams{
		Port:          builderOpts.CollectorGRPCPort,
		Handler:       grpcHandler,
		TLSConfig:     builderOpts.TLS,
		SamplingStore: c.strategyStore,
		Logger:        c.logger,
	}); err != nil {
		c.logger.Fatal("Could not start gRPC collector", zap.Error(err))
	} else {
		c.grpcServer = grpcServer
	}

	if httpServer, err := c.StartHTTPServer(&HTTPServerParams{
		Port:            builderOpts.CollectorZipkinHTTPPort,
		Handler:         jaegerBatchesHandler,
		RecoveryHandler: recoveryHandler,
		HealthCheck:     c.hCheck,
		MetricsFactory:  c.metricsFactory,
		SamplingStore:   c.strategyStore,
		Logger:          c.logger,
	}); err != nil {
		c.logger.Fatal("Could not start the HTTP server", zap.Error(err))
	} else {
		c.hServer = httpServer
	}

	if zkServer, err := c.StartZipkinServer(&ZipkinServerParams{
		Port:            builderOpts.CollectorZipkinHTTPPort,
		Handler:         zipkinSpansHandler,
		RecoveryHandler: recoveryHandler,
		AllowedHeaders:  builderOpts.CollectorZipkinAllowedHeaders,
		AllowedOrigins:  builderOpts.CollectorZipkinAllowedOrigins,
		Logger:          c.logger,
	}); err != nil {
		c.logger.Fatal("Could not start the Zipkin server", zap.Error(err))
	} else {
		c.zkServer = zkServer
	}

	return nil
}

// ThriftServerParams to construct a new Jaeger Collector Thrift Server
type ThriftServerParams struct {
	JaegerBatchesHandler handler.JaegerBatchesHandler
	ZipkinSpansHandler   handler.ZipkinSpansHandler
	StrategyStore        strategystore.StrategyStore
	ServiceName          string
	Port                 int
	Logger               *zap.Logger
}

// StartThriftServer based on the given parameters
func (c *Collector) StartThriftServer(params *ThriftServerParams) (*tchannel.Channel, error) {
	var tchServer *tchannel.Channel
	var err error

	if tchServer, err = tchannel.NewChannel(params.ServiceName, &tchannel.ChannelOptions{}); err != nil {
		params.Logger.Fatal("Unable to create new TChannel", zap.Error(err))
		return nil, err
	}

	server := thrift.NewServer(tchServer)
	batchHandler := handler.NewTChannelHandler(params.JaegerBatchesHandler, params.ZipkinSpansHandler)
	server.Register(jc.NewTChanCollectorServer(batchHandler))
	server.Register(zc.NewTChanZipkinCollectorServer(batchHandler))
	server.Register(sc.NewTChanSamplingManagerServer(sampling.NewHandler(params.StrategyStore)))

	portStr := ":" + strconv.Itoa(params.Port)
	listener, err := net.Listen("tcp", portStr)
	if err != nil {
		params.Logger.Fatal("Unable to start listening on channel", zap.Error(err))
		return nil, err
	}

	params.Logger.Info("Starting jaeger-collector TChannel server", zap.Int("port", params.Port))
	params.Logger.Warn("TChannel has been deprecated and will be removed in a future release")

	if err = tchServer.Serve(listener); err != nil {
		return nil, err
	}

	return tchServer, nil
}

// GRPCServerParams to construct a new Jaeger Collector gRPC Server
type GRPCServerParams struct {
	TLSConfig     tlscfg.Options
	Port          int
	Handler       *handler.GRPCHandler
	SamplingStore strategystore.StrategyStore
	Logger        *zap.Logger
}

// StartGRPCServer based on the given parameters
func (c *Collector) StartGRPCServer(params *GRPCServerParams) (*grpc.Server, error) {
	var server *grpc.Server
	if params.TLSConfig.Enabled {
		// user requested a server with TLS, setup creds
		tlsCfg, err := params.TLSConfig.Config()
		if err != nil {
			return nil, err
		}

		creds := credentials.NewTLS(tlsCfg)
		server = grpc.NewServer(grpc.Creds(creds))
	} else {
		// server without TLS
		server = grpc.NewServer()
	}

	_, err := grpcserver.StartGRPCCollector(params.Port, server, params.Handler, params.SamplingStore, params.Logger, func(err error) {
		params.Logger.Fatal("gRPC collector failed", zap.Error(err))
	})
	if err != nil {
		return nil, err
	}

	return server, err
}

// HTTPServerParams to construct a new Jaeger Collector HTTP Server
type HTTPServerParams struct {
	Port            int
	Handler         handler.JaegerBatchesHandler
	RecoveryHandler func(http.Handler) http.Handler
	SamplingStore   strategystore.StrategyStore
	MetricsFactory  metrics.Factory
	HealthCheck     *healthcheck.HealthCheck
	Logger          *zap.Logger
}

// StartHTTPServer based on the given parameters
func (c *Collector) StartHTTPServer(params *HTTPServerParams) (*http.Server, error) {
	r := mux.NewRouter()
	apiHandler := handler.NewAPIHandler(params.Handler)
	apiHandler.RegisterRoutes(r)

	cfgHandler := clientcfgHandler.NewHTTPHandler(clientcfgHandler.HTTPHandlerParams{
		ConfigManager: &clientcfgHandler.ConfigManager{
			SamplingStrategyStore: params.SamplingStore,
			// TODO provide baggage manager
		},
		MetricsFactory:         params.MetricsFactory,
		BasePath:               "/api",
		LegacySamplingEndpoint: false,
	})
	cfgHandler.RegisterRoutes(r)

	httpPortStr := ":" + strconv.Itoa(params.Port)
	params.Logger.Info("Starting jaeger-collector HTTP server", zap.String("http-host-port", httpPortStr))

	listener, err := net.Listen("tcp", httpPortStr)
	if err != nil {
		return nil, err
	}

	hServer := &http.Server{Addr: httpPortStr, Handler: params.RecoveryHandler(r)}
	go func(listener net.Listener, hServer *http.Server) {
		if err := hServer.Serve(listener); err != nil {
			if err != http.ErrServerClosed {
				params.Logger.Fatal("Could not start HTTP collector", zap.Error(err))
			}
		}
		params.HealthCheck.Set(healthcheck.Unavailable)
	}(listener, hServer)

	return hServer, nil
}

// ZipkinServerParams to construct a new Jaeger Collector Zipkin Server
type ZipkinServerParams struct {
	Port            int
	Handler         handler.ZipkinSpansHandler
	RecoveryHandler func(http.Handler) http.Handler
	AllowedOrigins  string
	AllowedHeaders  string
	HealthCheck     *healthcheck.HealthCheck
	Logger          *zap.Logger
}

// StartZipkinServer based on the given parameters
func (c *Collector) StartZipkinServer(params *ZipkinServerParams) (*http.Server, error) {
	var zkServer *http.Server

	if params.Port == 0 {
		return nil, nil
	}

	zHandler := zipkin.NewAPIHandler(params.Handler)
	r := mux.NewRouter()
	zHandler.RegisterRoutes(r)

	origins := strings.Split(strings.ReplaceAll(params.AllowedOrigins, " ", ""), ",")
	headers := strings.Split(strings.ReplaceAll(params.AllowedHeaders, " ", ""), ",")

	cors := cors.New(cors.Options{
		AllowedOrigins: origins,
		AllowedMethods: []string{"POST"}, // Allowing only POST, because that's the only handled one
		AllowedHeaders: headers,
	})

	httpPortStr := ":" + strconv.Itoa(params.Port)
	params.Logger.Info("Listening for Zipkin HTTP traffic", zap.Int("zipkin.http-port", params.Port))

	listener, err := net.Listen("tcp", httpPortStr)
	if err != nil {
		return nil, err
	}

	zkServer = &http.Server{Handler: cors.Handler(params.RecoveryHandler(r))}
	go func(listener net.Listener, zkServer *http.Server) {
		if err := zkServer.Serve(listener); err != nil {
			params.Logger.Fatal("Could not launch Zipkin server", zap.Error(err))
		}
		params.HealthCheck.Set(healthcheck.Unavailable)
	}(listener, zkServer)

	return zkServer, nil
}

// Close the component and all its underlying dependencies
func (c *Collector) Close() error {
	c.grpcServer.GracefulStop() // gRPC
	c.tchServer.Close()         // TChannel

	// HTTP server
	if c.hServer != nil {
		timeout, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := c.hServer.Shutdown(timeout)
		if err != nil {
			c.logger.Error("Failed to stop the main HTTP server", zap.Error(err))
		}
		defer cancel()
	}

	// Zipkin server
	if c.zkServer != nil {
		timeout, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := c.zkServer.Shutdown(timeout)
		if err != nil {
			c.logger.Error("Failed to stop the Zipkin server", zap.Error(err))
		}
		defer cancel()
	}

	// by now, we shouldn't have any in-flight requests anymore
	if c.spanWriter != nil {
		if closer, ok := c.spanWriter.(io.Closer); ok {
			err := closer.Close() // SpanWriter
			if err != nil {
				c.logger.Error("Failed to close span writer", zap.Error(err))
			}
		}
	}

	return nil
}
