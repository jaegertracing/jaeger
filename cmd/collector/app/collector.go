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
	"net/http"
	"time"

	"github.com/uber/jaeger-lib/metrics"
	"github.com/uber/tchannel-go"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	"github.com/jaegertracing/jaeger/cmd/collector/app/sampling/strategystore"
	"github.com/jaegertracing/jaeger/cmd/collector/app/server"
	"github.com/jaegertracing/jaeger/pkg/healthcheck"
	"github.com/jaegertracing/jaeger/pkg/recoveryhandler"
	"github.com/jaegertracing/jaeger/storage/spanstore"
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

	if tchServer, err := server.StartThriftServer(&server.ThriftServerParams{
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

	if grpcServer, err := server.StartGRPCServer(&server.GRPCServerParams{
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

	if httpServer, err := server.StartHTTPServer(&server.HTTPServerParams{
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

	if zkServer, err := server.StartZipkinServer(&server.ZipkinServerParams{
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

// Close the component and all its underlying dependencies
func (c *Collector) Close() error {
	// gRPC server
	if c.grpcServer != nil {
		c.grpcServer.GracefulStop()
	}

	// TChannel server
	if c.tchServer != nil {
		c.tchServer.Close()
	}

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
