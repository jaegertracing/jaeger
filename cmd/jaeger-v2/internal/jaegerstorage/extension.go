// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerstorage

import (
	"context"
	"fmt"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/internal/flags"
	queryApp "github.com/jaegertracing/jaeger/cmd/query/app"
	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	"github.com/jaegertracing/jaeger/pkg/jtracer"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
	"github.com/jaegertracing/jaeger/plugin/storage/memory"
	"github.com/jaegertracing/jaeger/storage"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	storageMetrics "github.com/jaegertracing/jaeger/storage/spanstore/metrics"
)

var _ extension.Extension = (*StorageExt)(nil)

type StorageExt struct {
	config    *Config
	logger    *zap.Logger
	factories map[string]storage.Factory
}

func newStorageExt(config *Config, otel component.TelemetrySettings) *StorageExt {
	return &StorageExt{
		config:    config,
		logger:    otel.Logger,
		factories: make(map[string]storage.Factory),
	}
}

func (s *StorageExt) Start(ctx context.Context, host component.Host) error {
	for name, mem := range s.config.Memory {
		if _, ok := s.factories[name]; ok {
			return fmt.Errorf("duplicate memory storage name %s", name)
		}
		s.factories[name] = memory.NewFactoryWithConfig(
			mem,
			metrics.NullFactory,
			s.logger.With(zap.String("storage_name", name)),
		)
	}
	return nil
}

func (s *StorageExt) Shutdown(ctx context.Context) error {
	return nil
}

func (s *StorageExt) GetStorageFactory(name string) storage.Factory {
	return s.factories[name]
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
