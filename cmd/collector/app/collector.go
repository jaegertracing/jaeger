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
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	"github.com/jaegertracing/jaeger/cmd/collector/app/processor"
	"github.com/jaegertracing/jaeger/cmd/collector/app/sampling/strategystore"
	"github.com/jaegertracing/jaeger/cmd/collector/app/server"
	"github.com/jaegertracing/jaeger/pkg/healthcheck"
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
	spanProcessor  processor.SpanProcessor
	spanHandlers   *SpanHandlers

	// state, read only
	hServer    *http.Server
	zkServer   *http.Server
	grpcServer *grpc.Server
	tlsCloser  io.Closer
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

	c.spanProcessor = handlerBuilder.BuildSpanProcessor()
	c.spanHandlers = handlerBuilder.BuildHandlers(c.spanProcessor)

	grpcServer, err := server.StartGRPCServer(&server.GRPCServerParams{
		HostPort:      builderOpts.CollectorGRPCHostPort,
		Handler:       c.spanHandlers.GRPCHandler,
		TLSConfig:     builderOpts.TLS,
		SamplingStore: c.strategyStore,
		Logger:        c.logger,
	})
	if err != nil {
		return fmt.Errorf("could not start gRPC collector %w", err)
	}
	c.grpcServer = grpcServer

	httpServer, err := server.StartHTTPServer(&server.HTTPServerParams{
		HostPort:       builderOpts.CollectorHTTPHostPort,
		Handler:        c.spanHandlers.JaegerBatchesHandler,
		HealthCheck:    c.hCheck,
		MetricsFactory: c.metricsFactory,
		SamplingStore:  c.strategyStore,
		Logger:         c.logger,
	})
	if err != nil {
		return fmt.Errorf("could not start the HTTP server %w", err)
	}
	c.hServer = httpServer

	c.tlsCloser = &builderOpts.TLS
	zkServer, err := server.StartZipkinServer(&server.ZipkinServerParams{
		HostPort:       builderOpts.CollectorZipkinHTTPHostPort,
		Handler:        c.spanHandlers.ZipkinSpansHandler,
		HealthCheck:    c.hCheck,
		AllowedHeaders: builderOpts.CollectorZipkinAllowedHeaders,
		AllowedOrigins: builderOpts.CollectorZipkinAllowedOrigins,
		Logger:         c.logger,
		MetricsFactory: c.metricsFactory,
	})
	if err != nil {
		return fmt.Errorf("could not start the Zipkin server %w", err)
	}
	c.zkServer = zkServer

	c.publishOpts(builderOpts)

	return nil
}

func (c *Collector) publishOpts(cOpts *CollectorOptions) {
	internalFactory := c.metricsFactory.Namespace(metrics.NSOptions{Name: "internal"})
	internalFactory.Gauge(metrics.Options{Name: collectorNumWorkers}).Update(int64(cOpts.NumWorkers))
	internalFactory.Gauge(metrics.Options{Name: collectorQueueSize}).Update(int64(cOpts.QueueSize))
}

// Close the component and all its underlying dependencies
func (c *Collector) Close() error {
	// gRPC server
	if c.grpcServer != nil {
		c.grpcServer.GracefulStop()
	}

	// HTTP server
	if c.hServer != nil {
		timeout, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := c.hServer.Shutdown(timeout); err != nil {
			c.logger.Fatal("failed to stop the main HTTP server", zap.Error(err))
		}
		defer cancel()
	}

	// Zipkin server
	if c.zkServer != nil {
		timeout, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := c.zkServer.Shutdown(timeout); err != nil {
			c.logger.Fatal("failed to stop the Zipkin server", zap.Error(err))
		}
		defer cancel()
	}

	if err := c.spanProcessor.Close(); err != nil {
		c.logger.Error("failed to close span processor.", zap.Error(err))
	}

	if err := c.tlsCloser.Close(); err != nil {
		c.logger.Error("failed to close TLS certificate watcher", zap.Error(err))
	}

	return nil
}

// SpanHandlers returns span handlers used by the Collector.
func (c *Collector) SpanHandlers() *SpanHandlers {
	return c.spanHandlers
}
