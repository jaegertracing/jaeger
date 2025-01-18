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
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage"
	queryApp "github.com/jaegertracing/jaeger/cmd/query/app"
	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	v2querysvc "github.com/jaegertracing/jaeger/cmd/query/app/querysvc/v2/querysvc"
	"github.com/jaegertracing/jaeger/pkg/jtracer"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/telemetry"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
	"github.com/jaegertracing/jaeger/plugin/metricstore/disabled"
	"github.com/jaegertracing/jaeger/storage/metricstore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	"github.com/jaegertracing/jaeger/storage_v2/depstore"
	"github.com/jaegertracing/jaeger/storage_v2/tracestore"
	"github.com/jaegertracing/jaeger/storage_v2/v1adapter"
)

var (
	_ extension.Extension             = (*server)(nil)
	_ extensioncapabilities.Dependent = (*server)(nil)
)

type server struct {
	config      *Config
	server      *queryApp.Server
	telset      component.TelemetrySettings
	closeTracer func(ctx context.Context) error
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
	// TODO OTel-collector does not initialize the tracer currently
	// https://github.com/open-telemetry/opentelemetry-collector/issues/7532
	//nolint
	tracerProvider, err := jtracer.New("jaeger")
	if err != nil {
		return fmt.Errorf("could not initialize a tracer: %w", err)
	}
	// make sure to close the tracer if subsequent code exists with error
	success := false
	defer func(ctx context.Context) {
		if success {
			s.closeTracer = tracerProvider.Close
		} else {
			tracerProvider.Close(ctx)
		}
	}(ctx)

	telset := telemetry.FromOtelComponent(s.telset, host)
	telset.TracerProvider = tracerProvider.OTEL
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
	}
	v2opts := v2querysvc.QueryServiceOptions{
		MaxClockSkewAdjust: s.config.MaxClockSkewAdjust,
	}
	if err := s.addArchiveStorage(&opts, &v2opts, host); err != nil {
		return err
	}
	qs := querysvc.NewQueryService(traceReader, depReader, opts)
	v2qs := v2querysvc.NewQueryService(traceReader, depReader, v2opts)

	mqs, err := s.createMetricReader(host)
	if err != nil {
		return err
	}

	tm := tenancy.NewManager(&s.config.Tenancy)

	s.server, err = queryApp.NewServer(
		ctx,
		// TODO propagate healthcheck updates up to the collector's runtime
		qs,
		v2qs,
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
	v2opts *v2querysvc.QueryServiceOptions,
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

	v2opts.ArchiveTraceReader = traceReader
	v2opts.ArchiveTraceWriter = traceWriter

	spanReader, spanWriter := getV1Adapters(traceReader, traceWriter)

	opts.ArchiveSpanReader = spanReader
	opts.ArchiveSpanWriter = spanWriter

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

func getV1Adapters(
	reader tracestore.Reader,
	writer tracestore.Writer,
) (spanstore.Reader, spanstore.Writer) {
	v1Reader, ok := v1adapter.GetV1Reader(reader)
	if !ok {
		v1Reader = v1adapter.NewSpanReader(reader)
	}

	v1Writer, ok := v1adapter.GetV1Writer(writer)
	if !ok {
		v1Writer = v1adapter.NewSpanWriter(writer)
	}

	return v1Reader, v1Writer
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
