// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerquery

import (
	"context"
	"errors"
	"fmt"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componentstatus"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/extension/extensioncapabilities"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage"
	queryApp "github.com/jaegertracing/jaeger/cmd/query/app"
	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	"github.com/jaegertracing/jaeger/internal/metrics/otelmetrics"
	"github.com/jaegertracing/jaeger/pkg/jtracer"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/telemetery"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
	"github.com/jaegertracing/jaeger/plugin/metrics/disabled"
	"github.com/jaegertracing/jaeger/storage/metricsstore"
	"github.com/jaegertracing/jaeger/storage/metricsstore/metricstoremetrics"
	"github.com/jaegertracing/jaeger/storage/storagemetrics"
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
	mf := otelmetrics.NewFactory(s.telset.MeterProvider)
	baseFactory := mf.Namespace(metrics.NSOptions{Name: "jaeger"})
	queryMetricsFactory := baseFactory.Namespace(metrics.NSOptions{Name: "query"})
	f, err := jaegerstorage.GetStorageFactory(s.config.Storage.TracesPrimary, host)
	if err != nil {
		return fmt.Errorf("cannot find primary storage %s: %w", s.config.Storage.TracesPrimary, err)
	}
	f = storagemetrics.NewDecoratorFactory(f, queryMetricsFactory)

	spanReader, err := f.CreateSpanReader()
	if err != nil {
		return fmt.Errorf("cannot create span reader: %w", err)
	}

	depReader, err := f.CreateDependencyReader()
	if err != nil {
		return fmt.Errorf("cannot create dependencies reader: %w", err)
	}

	var opts querysvc.QueryServiceOptions
	if err := s.addArchiveStorage(&opts, host); err != nil {
		return err
	}
	qs := querysvc.NewQueryService(spanReader, depReader, opts)

	mqs, err := s.createMetricReader(host)
	if err != nil {
		return err
	}

	tm := tenancy.NewManager(&s.config.Tenancy)

	// TODO OTel-collector does not initialize the tracer currently
	// https://github.com/open-telemetry/opentelemetry-collector/issues/7532
	//nolint
	tracerProvider, err := jtracer.New("jaeger")
	if err != nil {
		return fmt.Errorf("could not initialize a tracer: %w", err)
	}
	s.closeTracer = tracerProvider.Close
	telset := telemetery.Setting{
		Logger:         s.telset.Logger,
		TracerProvider: tracerProvider.OTEL,
		Metrics:        queryMetricsFactory,
		ReportStatus: func(event *componentstatus.Event) {
			componentstatus.ReportStatus(host, event)
		},
		LeveledMeterProvider: s.telset.LeveledMeterProvider,
		Host:                 host,
	}

	s.server, err = queryApp.NewServer(
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

	return nil
}

func (s *server) addArchiveStorage(opts *querysvc.QueryServiceOptions, host component.Host) error {
	if s.config.Storage.TracesArchive == "" {
		s.telset.Logger.Info("Archive storage not configured")
		return nil
	}

	f, err := jaegerstorage.GetStorageFactory(s.config.Storage.TracesArchive, host)
	if err != nil {
		return fmt.Errorf("cannot find archive storage factory: %w", err)
	}

	if !opts.InitArchiveStorage(f, s.telset.Logger) {
		s.telset.Logger.Info("Archive storage not initialized")
	}
	return nil
}

func (s *server) createMetricReader(host component.Host) (metricsstore.Reader, error) {
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

	// Decorate the metrics reader with metrics instrumentation.
	mf := otelmetrics.NewFactory(s.telset.MeterProvider)
	mf = mf.Namespace(metrics.NSOptions{Name: "jaeger_metricstore"})
	return metricstoremetrics.NewReaderDecorator(metricsReader, mf), nil
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
