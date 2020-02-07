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
	options    *CollectorOptions
}

// CollectorParams should be used as argument to create a new Collector. All fields are required.
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
	var err error

	handlerBuilder := &SpanHandlerBuilder{
		SpanWriter:     c.spanWriter,
		CollectorOpts:  *builderOpts,
		Logger:         c.logger,
		MetricsFactory: c.metricsFactory,
	}

	c.options = builderOpts

	zipkinSpansHandler, jaegerBatchesHandler, grpcHandler := handlerBuilder.BuildHandlers()

	c.tchServer, err = tchannel.NewChannel(c.serviceName, &tchannel.ChannelOptions{})
	{
		if err != nil {
			c.logger.Fatal("Unable to create new TChannel", zap.Error(err))
		}
		server := thrift.NewServer(c.tchServer)
		batchHandler := handler.NewTChannelHandler(jaegerBatchesHandler, zipkinSpansHandler)
		server.Register(jc.NewTChanCollectorServer(batchHandler))
		server.Register(zc.NewTChanZipkinCollectorServer(batchHandler))
		server.Register(sc.NewTChanSamplingManagerServer(sampling.NewHandler(c.strategyStore)))
		portStr := ":" + strconv.Itoa(builderOpts.CollectorPort)
		listener, err := net.Listen("tcp", portStr)
		if err != nil {
			c.logger.Fatal("Unable to start listening on channel", zap.Error(err))
		}
		c.logger.Info("Starting jaeger-collector TChannel server", zap.Int("port", builderOpts.CollectorPort))
		c.logger.Warn("TChannel has been deprecated and will be removed in a future release")
		c.tchServer.Serve(listener)
	}

	c.grpcServer, err = startGRPCServer(builderOpts, grpcHandler, c.strategyStore, c.logger)
	if err != nil {
		c.logger.Fatal("Could not start gRPC collector", zap.Error(err))
	}

	{
		r := mux.NewRouter()
		apiHandler := handler.NewAPIHandler(jaegerBatchesHandler)
		apiHandler.RegisterRoutes(r)

		cfgHandler := clientcfgHandler.NewHTTPHandler(clientcfgHandler.HTTPHandlerParams{
			ConfigManager: &clientcfgHandler.ConfigManager{
				SamplingStrategyStore: c.strategyStore,
				// TODO provide baggage manager
			},
			MetricsFactory:         c.metricsFactory,
			BasePath:               "/api",
			LegacySamplingEndpoint: false,
		})
		cfgHandler.RegisterRoutes(r)

		recoveryHandler := recoveryhandler.NewRecoveryHandler(c.logger, true)
		httpHandler := recoveryHandler(r)

		httpPortStr := ":" + strconv.Itoa(builderOpts.CollectorHTTPPort)
		c.logger.Info("Starting jaeger-collector HTTP server", zap.String("http-host-port", httpPortStr))

		c.hServer = &http.Server{Addr: httpPortStr, Handler: httpHandler}
		go func() {
			if err := c.hServer.ListenAndServe(); err != nil {
				if err != http.ErrServerClosed {
					c.logger.Fatal("Could not launch service", zap.Error(err))
				}
			}
			c.hCheck.Set(healthcheck.Unavailable)
		}()

		c.zkServer = zipkinServer(c.logger, builderOpts.CollectorZipkinHTTPPort, builderOpts.CollectorZipkinAllowedOrigins, builderOpts.CollectorZipkinAllowedHeaders, zipkinSpansHandler, recoveryHandler)
		if c.zkServer != nil {
			go func() {
				if err := c.zkServer.ListenAndServe(); err != nil {
					c.logger.Fatal("Could not launch Zipkin server", zap.Error(err))
				}
				c.hCheck.Set(healthcheck.Unavailable)
			}()
		}

	}

	return nil
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

func startGRPCServer(
	opts *CollectorOptions,
	handler *handler.GRPCHandler,
	samplingStore strategystore.StrategyStore,
	logger *zap.Logger,
) (*grpc.Server, error) {
	var server *grpc.Server

	if opts.TLS.Enabled { // user requested a server with TLS, setup creds
		tlsCfg, err := opts.TLS.Config()
		if err != nil {
			return nil, err
		}

		creds := credentials.NewTLS(tlsCfg)
		server = grpc.NewServer(grpc.Creds(creds))
	} else { // server without TLS
		server = grpc.NewServer()
	}
	_, err := grpcserver.StartGRPCCollector(opts.CollectorGRPCPort, server, handler, samplingStore, logger, func(err error) {
		logger.Fatal("gRPC collector failed", zap.Error(err))
	})
	if err != nil {
		return nil, err
	}
	return server, err
}

func zipkinServer(
	logger *zap.Logger,
	zipkinPort int,
	allowedOrigins string,
	allowedHeaders string,
	zipkinSpansHandler handler.ZipkinSpansHandler,
	recoveryHandler func(http.Handler) http.Handler,
) *http.Server {
	if zipkinPort != 0 {
		zHandler := zipkin.NewAPIHandler(zipkinSpansHandler)
		r := mux.NewRouter()
		zHandler.RegisterRoutes(r)

		origins := strings.Split(strings.Replace(allowedOrigins, " ", "", -1), ",")
		headers := strings.Split(strings.Replace(allowedHeaders, " ", "", -1), ",")

		c := cors.New(cors.Options{
			AllowedOrigins: origins,
			AllowedMethods: []string{"POST"}, // Allowing only POST, because that's the only handled one
			AllowedHeaders: headers,
		})

		httpPortStr := ":" + strconv.Itoa(zipkinPort)
		logger.Info("Listening for Zipkin HTTP traffic", zap.Int("zipkin.http-port", zipkinPort))

		return &http.Server{Addr: httpPortStr, Handler: c.Handler(recoveryHandler(r))}
	}

	return nil
}
