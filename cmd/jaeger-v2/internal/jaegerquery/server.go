// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerquery

import (
	"context"
	"fmt"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/internal/flags"
	"github.com/jaegertracing/jaeger/cmd/jaeger-v2/internal/jaegerstorage"
	queryApp "github.com/jaegertracing/jaeger/cmd/query/app"
	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	"github.com/jaegertracing/jaeger/pkg/jtracer"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	storageMetrics "github.com/jaegertracing/jaeger/storage/spanstore/metrics"
)

var _ extension.Extension = (*server)(nil)

type server struct {
	config *Config
	logger *zap.Logger
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
	return nil
}

func (s *server) Shutdown(ctx context.Context) error {
	return nil
}

func startQuery(
	svc *flags.Service,
	qOpts *queryApp.QueryOptions,
	queryOpts *querysvc.QueryServiceOptions,
	spanReader spanstore.Reader,
	depReader dependencystore.Reader,
	metricsQueryService querysvc.MetricsQueryService,
	baseFactory metrics.Factory,
	tm *tenancy.Manager,
	jt *jtracer.JTracer,
) *queryApp.Server {
	spanReader = storageMetrics.NewReadMetricsDecorator(spanReader, baseFactory.Namespace(metrics.NSOptions{Name: "query"}))
	qs := querysvc.NewQueryService(spanReader, depReader, *queryOpts)
	server, err := queryApp.NewServer(svc.Logger, qs, metricsQueryService, qOpts, tm, jt)
	if err != nil {
		svc.Logger.Fatal("Could not create jaeger-query", zap.Error(err))
	}
	go func() {
		for s := range server.HealthCheckStatus() {
			svc.SetHealthCheckStatus(s)
		}
	}()
	if err := server.Start(); err != nil {
		svc.Logger.Fatal("Could not start jaeger-query", zap.Error(err))
	}

	return server
}
