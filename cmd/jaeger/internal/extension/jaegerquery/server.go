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

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage"
	queryApp "github.com/jaegertracing/jaeger/cmd/query/app"
	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	"github.com/jaegertracing/jaeger/pkg/jtracer"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/telemetry"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
	"github.com/jaegertracing/jaeger/plugin/metricstore/disabled"
	"github.com/jaegertracing/jaeger/storage/metricstore"
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

	// TODO currently v1 is still needed because of dependency storage
	v1Factory, err := jaegerstorage.GetStorageFactory(s.config.Storage.TracesPrimary, host)
	if err != nil {
		return fmt.Errorf("cannot find v1 factory for primary storage %s: %w", s.config.Storage.TracesPrimary, err)
	}
	f, err := jaegerstorage.GetStorageFactoryV2(s.config.Storage.TracesPrimary, host)
	if err != nil {
		return fmt.Errorf("cannot find v2 factory for primary storage %s: %w", s.config.Storage.TracesPrimary, err)
	}

	traceReader, err := f.CreateTraceReader()
	if err != nil {
		return fmt.Errorf("cannot create trace reader: %w", err)
	}

	depReader, err := v1Factory.CreateDependencyReader()
	if err != nil {
		return fmt.Errorf("cannot create dependencies reader: %w", err)
	}

	var opts querysvc.QueryServiceOptions
	// TODO archive storage still uses v1 factory
	if err := s.addArchiveStorage(&opts, host); err != nil {
		return err
	}
	qs := querysvc.NewQueryService(traceReader, depReader, opts)

	mqs, err := s.createMetricReader(host)
	if err != nil {
		return err
	}

	tm := tenancy.NewManager(&s.config.Tenancy)

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

	success = true
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
