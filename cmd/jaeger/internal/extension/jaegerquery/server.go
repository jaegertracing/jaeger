// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerquery

import (
	"context"
	"errors"
	"fmt"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/extension/extensioncapabilities"
	"go.opentelemetry.io/otel/trace"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"

	queryapp "github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage"
	"github.com/jaegertracing/jaeger/internal/jtracer"
	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/metricstore/disabled"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/telemetry"
	"github.com/jaegertracing/jaeger/internal/tenancy"
)

var (
	_ extension.Extension             = (*server)(nil)
	_ extensioncapabilities.Dependent = (*server)(nil)
	_ Extension                       = (*server)(nil)
)

type server struct {
	config      *Config
	server      *queryapp.Server
	telset      component.TelemetrySettings
	closeTracer func(ctx context.Context) error
	qs          *querysvc.QueryService
}

func newServer(config *Config, otel component.TelemetrySettings) *server {
	return &server{
		config: config,
		telset: otel,
	}
}

// Dependencies implements extensioncapabilities.Dependent
// to ensure this always starts after jaegerstorage extension.
func (*server) Dependencies() []component.ID {
	return []component.ID{jaegerstorage.ID}
}

func (s *server) Start(ctx context.Context, host component.Host) error {
	var tp trace.TracerProvider = nooptrace.NewTracerProvider()
	success := false
	if s.config.EnableTracing {
		// TODO OTel-collector does not initialize the tracer currently
		// https://github.com/open-telemetry/opentelemetry-collector/issues/7532
		//nolint
		tracerProvider, tracerCloser, err := jtracer.NewProvider(ctx, "jaeger")
		if err != nil {
			return fmt.Errorf("could not initialize a tracer: %w", err)
		}
		tp = tracerProvider
		// Store closer for tracer if this function exists successfully,
		// otherwise call the closer right away.
		defer func(ctx context.Context) {
			if success {
				s.closeTracer = tracerCloser
			} else {
				tracerCloser(ctx)
			}
		}(ctx)
	}

	telset := telemetry.FromOtelComponent(s.telset, host)
	telset.TracerProvider = tp
	telset.Metrics = telset.Metrics.
		Namespace(metrics.NSOptions{Name: "jaeger"}).
		Namespace(metrics.NSOptions{Name: "query"})
	tf, err := jaegerstorage.GetTraceStoreFactory(s.config.Storage.TracesPrimary, host)
	if err != nil {
		return fmt.Errorf("cannot find factory for trace storage %s: %w", s.config.Storage.TracesPrimary, err)
	}
	traceReader, err := tf.CreateTraceReader()
	if err != nil {
		return fmt.Errorf("cannot create trace reader: %w", err)
	}

	df, ok := tf.(depstore.Factory)
	if !ok {
		return fmt.Errorf("cannot find factory for dependency storage %s: %w", s.config.Storage.TracesPrimary, err)
	}
	depReader, err := df.CreateDependencyReader()
	if err != nil {
		return fmt.Errorf("cannot create dependencies reader: %w", err)
	}

	opts := querysvc.QueryServiceOptions{
		MaxClockSkewAdjust: s.config.MaxClockSkewAdjust,
		MaxTraceSize:       s.config.MaxTraceSize,
	}
	if err := s.addArchiveStorage(&opts, host); err != nil {
		return err
	}
	qs := querysvc.NewQueryService(traceReader, depReader, opts)
	s.qs = qs

	mqs, err := s.createMetricReader(host)
	if err != nil {
		return err
	}

	tm := tenancy.NewManager(&s.config.Tenancy)

	s.server, err = queryapp.NewServer(
		ctx,
		// TODO propagate healthcheck updates up to the collector's runtime
		qs,
		mqs,
		&s.config.QueryOptions,
		tm,
		telset,
	)
	if err != nil {
		return fmt.Errorf("could not create jaeger-query: %w", err)
	}

	if err := s.server.Start(ctx); err != nil {
		return fmt.Errorf("could not start jaeger-query: %w", err)
	}

	success = true
	return nil
}

func (s *server) addArchiveStorage(
	opts *querysvc.QueryServiceOptions,
	host component.Host,
) error {
	if s.config.Storage.TracesArchive == "" {
		s.telset.Logger.Info("Archive storage not configured")
		return nil
	}

	f, err := jaegerstorage.GetTraceStoreFactory(s.config.Storage.TracesArchive, host)
	if err != nil {
		return fmt.Errorf("cannot find traces archive storage factory: %w", err)
	}

	traceReader, traceWriter := s.initArchiveStorage(f)
	if traceReader == nil || traceWriter == nil {
		return nil
	}

	opts.ArchiveTraceReader = traceReader
	opts.ArchiveTraceWriter = traceWriter

	return nil
}

func (s *server) initArchiveStorage(f tracestore.Factory) (tracestore.Reader, tracestore.Writer) {
	reader, err := f.CreateTraceReader()
	if err != nil {
		s.telset.Logger.Error("Cannot init traces archive storage reader", zap.Error(err))
		return nil, nil
	}
	writer, err := f.CreateTraceWriter()
	if err != nil {
		s.telset.Logger.Error("Cannot init traces archive storage writer", zap.Error(err))
		return nil, nil
	}
	return reader, writer
}

func (s *server) createMetricReader(host component.Host) (metricstore.Reader, error) {
	if s.config.Storage.Metrics == "" {
		s.telset.Logger.Info("Metric storage not configured")
		return disabled.NewMetricsReader()
	}

	msf, err := jaegerstorage.GetMetricStorageFactory(s.config.Storage.Metrics, host)
	if err != nil {
		return nil, fmt.Errorf("cannot find metrics storage factory: %w", err)
	}

	metricsReader, err := msf.CreateMetricsReader()
	if err != nil {
		return nil, fmt.Errorf("cannot create metrics reader %w", err)
	}

	return metricsReader, nil
}

func (s *server) Shutdown(ctx context.Context) error {
	var errs []error
	if s.server != nil {
		errs = append(errs, s.server.Close())
	}
	if s.closeTracer != nil {
		errs = append(errs, s.closeTracer(ctx))
	}
	return errors.Join(errs...)
}

// QueryService returns the v2 query service instance.
func (s *server) QueryService() *querysvc.QueryService {
	return s.qs
}
