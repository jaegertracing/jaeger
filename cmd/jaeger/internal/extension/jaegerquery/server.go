// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerquery

import (
	"context"
	"fmt"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage"
	queryApp "github.com/jaegertracing/jaeger/cmd/query/app"
	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	"github.com/jaegertracing/jaeger/pkg/telemetery"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
	"github.com/jaegertracing/jaeger/plugin/metrics/disabled"
	"github.com/jaegertracing/jaeger/ports"
)

var (
	_ extension.Extension = (*server)(nil)
	_ extension.Dependent = (*server)(nil)
)

type server struct {
	config *Config
	server *queryApp.Server
	telset component.TelemetrySettings
}

func newServer(config *Config, otel component.TelemetrySettings) *server {
	return &server{
		config: config,
		telset: otel,
	}
}

// Dependencies implements extension.Dependent to ensure this always starts after jaegerstorage extension.
func (*server) Dependencies() []component.ID {
	return []component.ID{jaegerstorage.ID}
}

func (s *server) Start(_ context.Context, host component.Host) error {
	f, err := jaegerstorage.GetStorageFactory(s.config.TraceStoragePrimary, host)
	if err != nil {
		return fmt.Errorf("cannot find primary storage %s: %w", s.config.TraceStoragePrimary, err)
	}

	spanReader, err := f.CreateSpanReader()
	if err != nil {
		return fmt.Errorf("cannot create span reader: %w", err)
	}
	// TODO
	// spanReader = storageMetrics.NewReadMetricsDecorator(spanReader, baseFactory.Namespace(metrics.NSOptions{Name: "query"}))

	depReader, err := f.CreateDependencyReader()
	if err != nil {
		return fmt.Errorf("cannot create dependencies reader: %w", err)
	}

	var opts querysvc.QueryServiceOptions
	if err := s.addArchiveStorage(&opts, host); err != nil {
		return err
	}
	qs := querysvc.NewQueryService(spanReader, depReader, opts)
	metricsQueryService, _ := disabled.NewMetricsReader()
	tm := tenancy.NewManager(&s.config.Tenancy)

	// TODO OTel-collector does not initialize the tracer currently
	// https://github.com/open-telemetry/opentelemetry-collector/issues/7532
	//nolint

	internalTelset := telemetery.Setting{
		Logger:         s.telset.Logger,
		TracerProvider: s.telset.TracerProvider,
		ReportStatus:   s.telset.ReportStatus,
	}
	// TODO contextcheck linter complains about next line that context is not passed. It is not wrong.
	//nolint
	s.server, err = queryApp.NewServer(
		// TODO propagate healthcheck updates up to the collector's runtime
		qs,
		metricsQueryService,
		s.makeQueryOptions(),
		tm,
		internalTelset,
	)
	if err != nil {
		return fmt.Errorf("could not create jaeger-query: %w", err)
	}

	if err := s.server.Start(); err != nil {
		return fmt.Errorf("could not start jaeger-query: %w", err)
	}

	return nil
}

func (s *server) addArchiveStorage(opts *querysvc.QueryServiceOptions, host component.Host) error {
	if s.config.TraceStorageArchive == "" {
		s.telset.Logger.Info("Archive storage not configured")
		return nil
	}

	f, err := jaegerstorage.GetStorageFactory(s.config.TraceStorageArchive, host)
	if err != nil {
		return fmt.Errorf("cannot find archive storage factory: %w", err)
	}

	if !opts.InitArchiveStorage(f, s.telset.Logger) {
		s.telset.Logger.Info("Archive storage not initialized")
	}
	return nil
}

func (s *server) makeQueryOptions() *queryApp.QueryOptions {
	return &queryApp.QueryOptions{
		QueryOptionsBase: s.config.QueryOptionsBase,

		// TODO expose via config
		HTTPHostPort: ports.PortToHostPort(ports.QueryHTTP),
		GRPCHostPort: ports.PortToHostPort(ports.QueryGRPC),
	}
}

func (s *server) Shutdown(ctx context.Context) error {
	return s.server.Close()
}
