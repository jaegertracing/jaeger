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

	"go.opentelemetry.io/collector/receiver"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	"github.com/jaegertracing/jaeger/cmd/collector/app/flags"
	"github.com/jaegertracing/jaeger/cmd/collector/app/handler"
	"github.com/jaegertracing/jaeger/cmd/collector/app/processor"
	"github.com/jaegertracing/jaeger/cmd/collector/app/sampling/strategystore"
	"github.com/jaegertracing/jaeger/cmd/collector/app/server"
	"github.com/jaegertracing/jaeger/pkg/healthcheck"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

const (
	metricNumWorkers = "collector.num-workers"
	metricQueueSize  = "collector.queue-size"
)

// Collector returns the collector as a manageable unit of work
type Collector struct {
	// required to start a new collector
	serviceName    string
	logger         *zap.Logger
	metricsFactory metrics.Factory
	spanWriter     spanstore.Writer
	strategyStore  strategystore.StrategyStore
	aggregator     strategystore.Aggregator
	hCheck         *healthcheck.HealthCheck
	spanProcessor  processor.SpanProcessor
	spanHandlers   *SpanHandlers
	tenancyMgr     *tenancy.Manager

	// state, read only
	hServer                    *http.Server
	zkServer                   *http.Server
	grpcServer                 *grpc.Server
	otlpReceiver               receiver.Traces
	tlsGRPCCertWatcherCloser   io.Closer
	tlsHTTPCertWatcherCloser   io.Closer
	tlsZipkinCertWatcherCloser io.Closer
}

// CollectorParams to construct a new Jaeger Collector.
type CollectorParams struct {
	ServiceName    string
	Logger         *zap.Logger
	MetricsFactory metrics.Factory
	SpanWriter     spanstore.Writer
	StrategyStore  strategystore.StrategyStore
	Aggregator     strategystore.Aggregator
	HealthCheck    *healthcheck.HealthCheck
	TenancyMgr     *tenancy.Manager
}

// New constructs a new collector component, ready to be started
func New(params *CollectorParams) *Collector {
	return &Collector{
		serviceName:    params.ServiceName,
		logger:         params.Logger,
		metricsFactory: params.MetricsFactory,
		spanWriter:     params.SpanWriter,
		strategyStore:  params.StrategyStore,
		aggregator:     params.Aggregator,
		hCheck:         params.HealthCheck,
		tenancyMgr:     params.TenancyMgr,
	}
}

// Start the component and underlying dependencies
func (c *Collector) Start(options *flags.CollectorOptions) error {
	handlerBuilder := &SpanHandlerBuilder{
		SpanWriter:     c.spanWriter,
		CollectorOpts:  options,
		Logger:         c.logger,
		MetricsFactory: c.metricsFactory,
		TenancyMgr:     c.tenancyMgr,
	}

	var additionalProcessors []ProcessSpan
	if c.aggregator != nil {
		additionalProcessors = append(additionalProcessors, handleRootSpan(c.aggregator, c.logger))
	}

	c.spanProcessor = handlerBuilder.BuildSpanProcessor(additionalProcessors...)
	c.spanHandlers = handlerBuilder.BuildHandlers(c.spanProcessor)

	grpcServer, err := server.StartGRPCServer(&server.GRPCServerParams{
		HostPort:                options.GRPC.HostPort,
		Handler:                 c.spanHandlers.GRPCHandler,
		TLSConfig:               options.GRPC.TLS,
		SamplingStore:           c.strategyStore,
		Logger:                  c.logger,
		MaxReceiveMessageLength: options.GRPC.MaxReceiveMessageLength,
		MaxConnectionAge:        options.GRPC.MaxConnectionAge,
		MaxConnectionAgeGrace:   options.GRPC.MaxConnectionAgeGrace,
	})
	if err != nil {
		return fmt.Errorf("could not start gRPC server: %w", err)
	}
	c.grpcServer = grpcServer

	httpServer, err := server.StartHTTPServer(&server.HTTPServerParams{
		HostPort:       options.HTTP.HostPort,
		Handler:        c.spanHandlers.JaegerBatchesHandler,
		TLSConfig:      options.HTTP.TLS,
		HealthCheck:    c.hCheck,
		MetricsFactory: c.metricsFactory,
		SamplingStore:  c.strategyStore,
		Logger:         c.logger,
	})
	if err != nil {
		return fmt.Errorf("could not start HTTP server: %w", err)
	}
	c.hServer = httpServer

	c.tlsGRPCCertWatcherCloser = &options.GRPC.TLS
	c.tlsHTTPCertWatcherCloser = &options.HTTP.TLS
	c.tlsZipkinCertWatcherCloser = &options.Zipkin.TLS
	zkServer, err := server.StartZipkinServer(&server.ZipkinServerParams{
		HostPort:       options.Zipkin.HTTPHostPort,
		Handler:        c.spanHandlers.ZipkinSpansHandler,
		TLSConfig:      options.Zipkin.TLS,
		HealthCheck:    c.hCheck,
		CORSConfig:     options.Zipkin.CORS,
		Logger:         c.logger,
		MetricsFactory: c.metricsFactory,
		KeepAlive:      options.Zipkin.KeepAlive,
	})
	if err != nil {
		return fmt.Errorf("could not start Zipkin server: %w", err)
	}
	c.zkServer = zkServer

	if options.OTLP.Enabled {
		otlpReceiver, err := handler.StartOTLPReceiver(options, c.logger, c.spanProcessor, c.tenancyMgr)
		if err != nil {
			return fmt.Errorf("could not start OTLP receiver: %w", err)
		}
		c.otlpReceiver = otlpReceiver
	}

	c.publishOpts(options)

	return nil
}

func (c *Collector) publishOpts(cOpts *flags.CollectorOptions) {
	internalFactory := c.metricsFactory.Namespace(metrics.NSOptions{Name: "internal"})
	internalFactory.Gauge(metrics.Options{Name: metricNumWorkers}).Update(int64(cOpts.NumWorkers))
	internalFactory.Gauge(metrics.Options{Name: metricQueueSize}).Update(int64(cOpts.QueueSize))
}

// Close the component and all its underlying dependencies
func (c *Collector) Close() error {
	// Stop gRPC server
	if c.grpcServer != nil {
		c.grpcServer.GracefulStop()
	}

	// Stop HTTP server
	if c.hServer != nil {
		timeout, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := c.hServer.Shutdown(timeout); err != nil {
			c.logger.Fatal("failed to stop the main HTTP server", zap.Error(err))
		}
		defer cancel()
	}

	// Stop Zipkin server
	if c.zkServer != nil {
		timeout, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := c.zkServer.Shutdown(timeout); err != nil {
			c.logger.Fatal("failed to stop the Zipkin server", zap.Error(err))
		}
		defer cancel()
	}

	// Stop OpenTelemetry OTLP receiver
	if c.otlpReceiver != nil {
		timeout, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := c.otlpReceiver.Shutdown(timeout); err != nil {
			c.logger.Fatal("failed to stop the OTLP receiver", zap.Error(err))
		}
		defer cancel()
	}

	if err := c.spanProcessor.Close(); err != nil {
		c.logger.Error("failed to close span processor.", zap.Error(err))
	}

	// aggregator does not exist for all strategy stores. only Close() if exists.
	if c.aggregator != nil {
		if err := c.aggregator.Close(); err != nil {
			c.logger.Error("failed to close aggregator.", zap.Error(err))
		}
	}

	// watchers actually never return errors from Close
	_ = c.tlsGRPCCertWatcherCloser.Close()
	_ = c.tlsHTTPCertWatcherCloser.Close()
	_ = c.tlsZipkinCertWatcherCloser.Close()

	return nil
}

// SpanHandlers returns span handlers used by the Collector.
func (c *Collector) SpanHandlers() *SpanHandlers {
	return c.spanHandlers
}
