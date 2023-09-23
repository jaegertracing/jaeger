// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerquery

import (
	"context"
	"fmt"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/jaeger-v2/internal/jaegerstorage"
	queryApp "github.com/jaegertracing/jaeger/cmd/query/app"
	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	"github.com/jaegertracing/jaeger/pkg/jtracer"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
	"github.com/jaegertracing/jaeger/plugin/metrics/disabled"
	"github.com/jaegertracing/jaeger/ports"
)

var _ extension.Extension = (*server)(nil)

type server struct {
	config *Config
	logger *zap.Logger
	server *queryApp.Server
}

func newServer(config *Config, otel component.TelemetrySettings) *server {
	return &server{
		config: config,
		logger: otel.Logger,
	}
}

func (s *server) Start(ctx context.Context, host component.Host) error {
	var storageExt component.Component
	for id, ext := range host.GetExtensions() {
		if id.Type() == jaegerstorage.ComponentType {
			storageExt = ext
			break
		}
	}
	if storageExt == nil {
		return fmt.Errorf(
			"cannot find extension '%s'. Make sure it comes before '%s' in the config",
			jaegerstorage.ComponentType,
			typeStr,
		)
	}
	ext := storageExt.(*jaegerstorage.StorageExt)
	f := ext.GetStorageFactory(s.config.TraceStorage)
	if f == nil {
		return fmt.Errorf("cannot find trace_storage named '%s'", s.config.TraceStorage)
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

	qs := querysvc.NewQueryService(spanReader, depReader, querysvc.QueryServiceOptions{})
	metricsQueryService, _ := disabled.NewMetricsReader()
	tm := tenancy.NewManager(&s.config.Tenancy)

	s.server, err = queryApp.NewServer(
		s.logger,
		qs,
		metricsQueryService,
		makeQueryOptions(),
		tm,
		jtracer.NoOp(),
	)
	if err != nil {
		return fmt.Errorf("could not create jaeger-query: %w", err)
	}

	if err := s.server.Start(); err != nil {
		return fmt.Errorf("could not start jaeger-query: %w", err)
	}

	return nil
}

func makeQueryOptions() *queryApp.QueryOptions {
	return &queryApp.QueryOptions{
		// TODO
		HTTPHostPort: ports.PortToHostPort(ports.QueryHTTP),
		GRPCHostPort: ports.PortToHostPort(ports.QueryGRPC),
	}
}

func (s *server) Shutdown(ctx context.Context) error {
	if s.server != nil {
		return s.server.Close()
	}
	return nil
}
