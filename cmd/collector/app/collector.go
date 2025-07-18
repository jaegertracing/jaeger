// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"go.opentelemetry.io/collector/receiver"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/cmd/collector/app/flags"
	"github.com/jaegertracing/jaeger/cmd/collector/app/handler"
	"github.com/jaegertracing/jaeger/cmd/collector/app/processor"
	"github.com/jaegertracing/jaeger/cmd/collector/app/server"
	"github.com/jaegertracing/jaeger/internal/healthcheck"
	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/safeexpvar"
	"github.com/jaegertracing/jaeger/internal/sampling/samplingstrategy"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/tenancy"
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
	traceWriter        tracestore.Writer
	samplingProvider   samplingstrategy.Provider
	samplingAggregator samplingstrategy.Aggregator
	hCheck             *healthcheck.HealthCheck
	spanProcessor      processor.SpanProcessor
	spanHandlers       *SpanHandlers
	tenancyMgr         *tenancy.Manager

	// state, read only
	hServer        *http.Server
	grpcServer     *grpc.Server
	otlpReceiver   receiver.Traces
	zipkinReceiver receiver.Traces

	collectorSpanMetrics *CollectorSpanMetrics
}

// CollectorParams to construct a new Jaeger Collector.
type CollectorParams struct {
	ServiceName        string
	Logger             *zap.Logger
	MetricsFactory     metrics.Factory
	TraceWriter        tracestore.Writer
	SamplingProvider   samplingstrategy.Provider
	SamplingAggregator samplingstrategy.Aggregator
	HealthCheck        *healthcheck.HealthCheck
	TenancyMgr         *tenancy.Manager
}

func New(params *CollectorParams) *Collector {
	return &Collector{
		serviceName:        params.ServiceName,
		logger:             params.Logger,
		metricsFactory:     params.MetricsFactory,
		traceWriter:        params.TraceWriter,
		samplingProvider:   params.SamplingProvider,
		samplingAggregator: params.SamplingAggregator,
		hCheck:             params.HealthCheck,
		tenancyMgr:         params.TenancyMgr,
		collectorSpanMetrics: NewCollectorSpanMetrics(params.MetricsFactory),
	}
}

// Start the component and underlying dependencies
func (c *Collector) Start(options *flags.CollectorOptions) error {
	handlerBuilder := &SpanHandlerBuilder{
		TraceWriter:    c.traceWriter,
		CollectorOpts:  options,
		Logger:         c.logger,
		MetricsFactory: c.metricsFactory,
		TenancyMgr:     c.tenancyMgr,
	}

	var additionalProcessors []ProcessSpan
	if c.samplingAggregator != nil {
		additionalProcessors = append(additionalProcessors, ProcessSpan(func(span *model.Span, _ string) {
			c.samplingAggregator.HandleRootSpan(span)
		}))
	}

	spanProcessor, err := handlerBuilder.BuildSpanProcessor(additionalProcessors...)
	if err != nil {
		return fmt.Errorf("could not create span processor: %w", err)
	}

	c.spanProcessor = NewMetricsReportingSpanProcessor(spanProcessor, c.collectorSpanMetrics)

	c.spanHandlers = handlerBuilder.BuildHandlers(c.spanProcessor)

	grpcServer, err := server.StartGRPCServer(&server.GRPCServerParams{
		Handler:          c.spanHandlers.GRPCHandler,
		SamplingProvider: c.samplingProvider,
		Logger:           c.logger,
		ServerConfig:     options.GRPC,
	})
	if err != nil {
		return fmt.Errorf("could not start gRPC server: %w", err)
	}
	c.grpcServer = grpcServer

	httpServer, err := server.StartHTTPServer(&server.HTTPServerParams{
		ServerConfig:     options.HTTP,
		Handler:          c.spanHandlers.JaegerBatchesHandler,
		HealthCheck:      c.hCheck,
		MetricsFactory:   c.metricsFactory,
		SamplingProvider: c.samplingProvider,
		Logger:           c.logger,
	})
	if err != nil {
		return fmt.Errorf("could not start HTTP server: %w", err)
	}
	c.hServer = httpServer

	if options.Zipkin.Endpoint == "" {
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

	return nil
}

// SpanHandlers returns span handlers used by the Collector.
func (c *Collector) SpanHandlers() *SpanHandlers {
	return c.spanHandlers
}

type CollectorSpanMetrics struct {
	factory        metrics.Factory
	receivedMutex  sync.Mutex
	rejectedMutex  sync.Mutex
	receivedBySvc  map[string]metrics.Counter
	rejectedBySvc  map[string]metrics.Counter
}

func NewCollectorSpanMetrics(factory metrics.Factory) *CollectorSpanMetrics {
	return &CollectorSpanMetrics{
		factory:       factory,
		receivedBySvc: make(map[string]metrics.Counter),
		rejectedBySvc: make(map[string]metrics.Counter),
	}
}

func (m *CollectorSpanMetrics) incReceived(svc string) {
    m.incMetric(svc, m.receivedBySvc, "jaeger_collector_spans_received_total")
}

func (m *CollectorSpanMetrics) incRejected(svc string) {
    m.incMetric(svc, m.rejectedBySvc, "jaeger_collector_spans_rejected_total")
}

func (m *CollectorSpanMetrics) incMetric(svc string, counterMap map[string]metrics.Counter, metricName string) {
    m.receivedMutex.Lock()
    defer m.receivedMutex.Unlock()

    if svc == "" {
        svc = "unknown-service"
    }

    counter, ok := counterMap[svc]
    if !ok {
        counter = m.factory.Counter(metrics.Options{
			Name: metricName,
			Help: fmt.Sprintf("Number of spans %s by the collector by service.", metricName),
			Tags: map[string]string{"svc": svc},
		})		
        counterMap[svc] = counter
    }
    counter.Inc(1)
}


type metricsReportingSpanProcessor struct {
	wrapped processor.SpanProcessor
	metrics *CollectorSpanMetrics
}

func NewMetricsReportingSpanProcessor(wrapped processor.SpanProcessor, m *CollectorSpanMetrics) processor.SpanProcessor {
	return &metricsReportingSpanProcessor{wrapped: wrapped, metrics: m}
}

func (m *metricsReportingSpanProcessor) ProcessSpans(ctx context.Context, batch processor.Batch) ([]bool, error) {
	var spans []*model.Span
	if s, ok := interface{}(batch).([]*model.Span); ok {
		spans = s
	} else {
		spans = []*model.Span{}
	}

	for _, span := range spans {
		svcName := span.GetProcess().GetServiceName()
		m.metrics.incReceived(svcName)
	}

	results, err := m.wrapped.ProcessSpans(ctx, batch)
	if err != nil {
		for _, span := range spans {
			svcName := span.GetProcess().GetServiceName()
			m.metrics.incRejected(svcName)
		}
	}
	return results, err
}

func (m *metricsReportingSpanProcessor) Close() error {
	return m.wrapped.Close()
}
