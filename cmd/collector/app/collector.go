// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

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
	"github.com/jaegertracing/jaeger/cmd/collector/app/sampling/samplingstrategy"
	"github.com/jaegertracing/jaeger/cmd/collector/app/server"
	"github.com/jaegertracing/jaeger/internal/safeexpvar"
	"github.com/jaegertracing/jaeger/model"
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
	serviceName        string
	logger             *zap.Logger
	metricsFactory     metrics.Factory
	spanWriter         spanstore.Writer
	samplingProvider   samplingstrategy.Provider
	samplingAggregator samplingstrategy.Aggregator
	hCheck             *healthcheck.HealthCheck
	spanProcessor      processor.SpanProcessor
	spanHandlers       *SpanHandlers
	tenancyMgr         *tenancy.Manager

	// state, read only
	hServer                    *http.Server
	grpcServer                 *grpc.Server
	otlpReceiver               receiver.Traces
	zipkinReceiver             receiver.Traces
	tlsGRPCCertWatcherCloser   io.Closer
	tlsHTTPCertWatcherCloser   io.Closer
	tlsZipkinCertWatcherCloser io.Closer
}

// CollectorParams to construct a new Jaeger Collector.
type CollectorParams struct {
	ServiceName        string
	Logger             *zap.Logger
	MetricsFactory     metrics.Factory
	SpanWriter         spanstore.Writer
	SamplingProvider   samplingstrategy.Provider
	SamplingAggregator samplingstrategy.Aggregator
	HealthCheck        *healthcheck.HealthCheck
	TenancyMgr         *tenancy.Manager
}

// New constructs a new collector component, ready to be started
func New(params *CollectorParams) *Collector {
	return &Collector{
		serviceName:        params.ServiceName,
		logger:             params.Logger,
		metricsFactory:     params.MetricsFactory,
		spanWriter:         params.SpanWriter,
		samplingProvider:   params.SamplingProvider,
		samplingAggregator: params.SamplingAggregator,
		hCheck:             params.HealthCheck,
		tenancyMgr:         params.TenancyMgr,
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
	if c.samplingAggregator != nil {
		additionalProcessors = append(additionalProcessors, func(span *model.Span, _ /* tenant */ string) {
			c.samplingAggregator.HandleRootSpan(span, c.logger)
		})
	}

	c.spanProcessor = handlerBuilder.BuildSpanProcessor(additionalProcessors...)
	c.spanHandlers = handlerBuilder.BuildHandlers(c.spanProcessor)

	grpcServer, err := server.StartGRPCServer(&server.GRPCServerParams{
		HostPort:                options.GRPC.HostPort,
		Handler:                 c.spanHandlers.GRPCHandler,
		TLSConfig:               options.GRPC.TLS,
		SamplingProvider:        c.samplingProvider,
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
		HostPort:         options.HTTP.HostPort,
		Handler:          c.spanHandlers.JaegerBatchesHandler,
		TLSConfig:        options.HTTP.TLS,
		HealthCheck:      c.hCheck,
		MetricsFactory:   c.metricsFactory,
		SamplingProvider: c.samplingProvider,
		Logger:           c.logger,
	})
	if err != nil {
		return fmt.Errorf("could not start HTTP server: %w", err)
	}
	c.hServer = httpServer

	c.tlsGRPCCertWatcherCloser = &options.GRPC.TLS
	c.tlsHTTPCertWatcherCloser = &options.HTTP.TLS
	c.tlsZipkinCertWatcherCloser = &options.Zipkin.TLS

	if options.Zipkin.HTTPHostPort == "" {
		c.logger.Info("Not listening for Zipkin HTTP traffic, port not configured")
	} else {
		zipkinReceiver, err := handler.StartZipkinReceiver(options, c.logger, c.spanProcessor, c.tenancyMgr)
		if err != nil {
			return fmt.Errorf("could not start Zipkin receiver: %w", err)
		}
		c.zipkinReceiver = zipkinReceiver
	}

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

func (*Collector) publishOpts(cOpts *flags.CollectorOptions) {
	safeexpvar.SetInt(metricNumWorkers, int64(cOpts.NumWorkers))
	//nolint: gosec // G115
	safeexpvar.SetInt(metricQueueSize, int64(cOpts.QueueSize))
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

	// Stop Zipkin receiver
	if c.zipkinReceiver != nil {
		timeout, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := c.zipkinReceiver.Shutdown(timeout); err != nil {
			c.logger.Fatal("failed to stop the Zipkin receiver", zap.Error(err))
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
	if c.samplingAggregator != nil {
		if err := c.samplingAggregator.Close(); err != nil {
			c.logger.Error("failed to close aggregator.", zap.Error(err))
		}
	}

	// watchers actually never return errors from Close
	if c.tlsGRPCCertWatcherCloser != nil {
		_ = c.tlsGRPCCertWatcherCloser.Close()
	}
	if c.tlsHTTPCertWatcherCloser != nil {
		_ = c.tlsHTTPCertWatcherCloser.Close()
	}
	if c.tlsZipkinCertWatcherCloser != nil {
		_ = c.tlsZipkinCertWatcherCloser.Close()
	}

	return nil
}

// SpanHandlers returns span handlers used by the Collector.
func (c *Collector) SpanHandlers() *SpanHandlers {
	return c.spanHandlers
}
